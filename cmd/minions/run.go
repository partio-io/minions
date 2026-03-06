package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"github.com/spf13/cobra"

	"github.com/partio-io/minions/internal/checks"
	"github.com/partio-io/minions/internal/claude"
	"github.com/partio-io/minions/internal/pr"
	"github.com/partio-io/minions/internal/prompt"
	"github.com/partio-io/minions/internal/task"
	"github.com/partio-io/minions/internal/worktree"
)

func newRunCmd() *cobra.Command {
	var dryRun bool
	var parallel int

	cmd := &cobra.Command{
		Use:   "run <path>",
		Short: "Execute task specs end-to-end",
		Long:  `Execute one or more task YAML files. Creates worktrees, runs Claude Code, validates checks, and creates PRs.`,
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := args[0]

			if cfg.DryRun {
				dryRun = true
			}

			workspaceRoot := cfg.WorkspaceRoot
			if workspaceRoot == "" {
				// Default: parent of the directory containing the binary or cwd
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("getting working directory: %w", err)
				}
				workspaceRoot = filepath.Dir(wd)
			}

			// Load tasks
			var tasks []*task.Task
			info, err := os.Stat(target)
			if err != nil {
				return fmt.Errorf("cannot access %s: %w", target, err)
			}

			if info.IsDir() {
				tasks, err = task.LoadDir(target)
				if err != nil {
					return err
				}
			} else {
				t, err := task.LoadFile(target)
				if err != nil {
					return err
				}
				tasks = []*task.Task{t}
			}

			if len(tasks) == 0 {
				fmt.Println("No task files found in", target)
				return nil
			}

			fmt.Printf("Found %d task(s) to process\n", len(tasks))
			fmt.Printf("Workspace root: %s\n", workspaceRoot)
			fmt.Printf("Parallel: %d\n", parallel)
			fmt.Printf("Dry run: %v\n\n", dryRun)

			if parallel <= 1 {
				for _, t := range tasks {
					if err := executeTask(t, workspaceRoot, dryRun); err != nil {
						slog.Error("task failed", "task", t.ID, "error", err)
					}
				}
			} else {
				runParallel(tasks, workspaceRoot, dryRun, parallel)
			}

			fmt.Println("==========================================")
			fmt.Println("All tasks complete.")
			fmt.Println("==========================================")
			return nil
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview generated prompt without executing")
	cmd.Flags().IntVar(&parallel, "parallel", 1, "max parallel task executions")

	return cmd
}

func executeTask(t *task.Task, workspaceRoot string, dryRun bool) error {
	fmt.Println("==========================================")
	fmt.Printf("TASK: %s\n", t.ID)
	fmt.Printf("TITLE: %s\n", t.Title)
	fmt.Println("==========================================")

	// Build prompt
	fmt.Println("--- Building prompt ---")
	taskPrompt, err := prompt.BuildTask(t, workspaceRoot)
	if err != nil {
		return fmt.Errorf("building prompt: %w", err)
	}

	if dryRun {
		fmt.Println()
		fmt.Println("=== DRY RUN: Generated Prompt ===")
		fmt.Println(taskPrompt)
		fmt.Println("=== END PROMPT ===")
		fmt.Println()
		fmt.Println("Would create worktrees in:", t.TargetRepos)
		fmt.Printf("Would run: claude -p --max-turns %d\n", cfg.MaxTurns)
		return nil
	}

	// Read PR labels
	labelsCSV := "minion"
	if len(t.PRLabels) > 0 {
		labelsCSV = ""
		for i, l := range t.PRLabels {
			if i > 0 {
				labelsCSV += ","
			}
			labelsCSV += l
		}
	}

	// Create worktrees
	fmt.Println("--- Creating worktrees ---")
	var worktreePaths []string
	for _, repo := range t.TargetRepos {
		repoPath := filepath.Join(workspaceRoot, repo)
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			slog.Warn("repo not found, skipping", "path", repoPath)
			continue
		}
		wtPath, err := worktree.Create(repoPath, t.ID)
		if err != nil {
			return fmt.Errorf("creating worktree for %s: %w", repo, err)
		}
		worktreePaths = append(worktreePaths, wtPath)
		fmt.Printf("Created worktree: %s\n", wtPath)
	}

	if len(worktreePaths) == 0 {
		return fmt.Errorf("no worktrees created")
	}

	// Run Claude Code
	fmt.Println("--- Running Claude Code ---")
	err = claude.Run(claude.Opts{
		Prompt:   taskPrompt,
		CWD:      worktreePaths[0],
		MaxTurns: cfg.MaxTurns,
	})
	if err != nil {
		slog.Warn("claude exited with error", "error", err)
	}

	// Run checks
	fmt.Println("--- Running checks ---")
	allPass := true
	var failedOutput string
	for _, wtPath := range worktreePaths {
		output, err := checks.Run(wtPath)
		if err != nil {
			allPass = false
			failedOutput += output + "\n"
		}
	}

	// Retry on failure
	if !allPass {
		fmt.Println("--- Checks failed, retrying with error feedback ---")
		retryPrompt := fmt.Sprintf(`The following checks failed after your implementation. Please fix the issues:

%s

Fix the errors and ensure all checks pass. Do not introduce new changes beyond what's needed to fix the failures.`, failedOutput)

		_ = claude.Run(claude.Opts{
			Prompt:   retryPrompt,
			CWD:      worktreePaths[0],
			MaxTurns: 15,
		})

		// Re-run checks
		fmt.Println("--- Re-running checks after retry ---")
		allPass = true
		for _, wtPath := range worktreePaths {
			if _, err := checks.Run(wtPath); err != nil {
				allPass = false
			}
		}

		if !allPass {
			slog.Error("checks still failing after retry, skipping PR creation")
			cleanupWorktrees(t.TargetRepos, t.ID, workspaceRoot)
			return fmt.Errorf("checks failed for task %s", t.ID)
		}
	}

	// Create PRs
	fmt.Println("--- Creating PRs ---")
	_, err = pr.CreateAndLinkAll(t.ID, t.Title, workspaceRoot, labelsCSV, t.TargetRepos)
	if err != nil {
		slog.Error("PR creation failed", "error", err)
	}

	// Cleanup
	fmt.Println("--- Cleaning up worktrees ---")
	cleanupWorktrees(t.TargetRepos, t.ID, workspaceRoot)

	fmt.Printf("\nDONE: %s\n\n", t.ID)
	return nil
}

func cleanupWorktrees(repos []string, taskID, workspaceRoot string) {
	for _, repo := range repos {
		worktree.Cleanup(filepath.Join(workspaceRoot, repo), taskID)
	}
}

func runParallel(tasks []*task.Task, workspaceRoot string, dryRun bool, maxParallel int) {
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup

	for _, t := range tasks {
		wg.Add(1)
		sem <- struct{}{}

		go func(t *task.Task) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := executeTask(t, workspaceRoot, dryRun); err != nil {
				slog.Error("task failed", "task", t.ID, "error", err)
			}
		}(t)
	}

	wg.Wait()
}
