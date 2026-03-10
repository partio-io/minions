package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/partio-io/minions/internal/agent"
	appcontext "github.com/partio-io/minions/internal/context"
	"github.com/partio-io/minions/internal/pipeline"
	"github.com/partio-io/minions/internal/prompt"
	"github.com/partio-io/minions/internal/task"
)

func newRunCmd() *cobra.Command {
	var dryRun bool
	var parallel int
	var agentName string
	var prRef string
	var contextJSON string

	cmd := &cobra.Command{
		Use:   "run [path]",
		Short: "Execute task specs or agent types end-to-end",
		Long: `Execute one or more task YAML files, or run a named agent type.

Examples:
  minions run tasks/my-task.yaml
  minions run tasks/ --parallel 3
  minions run --agent doc-updater --pr partio-io/cli#42
  minions run --agent readme-updater --pr partio-io/app#42`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cfg.DryRun {
				dryRun = true
			}

			workspaceRoot := cfg.WorkspaceRoot
			if workspaceRoot == "" {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("getting working directory: %w", err)
				}
				workspaceRoot = filepath.Dir(wd)
			}

			// Agent mode: --agent flag specified
			if agentName != "" {
				return runAgent(agentName, prRef, contextJSON, workspaceRoot, dryRun)
			}

			// Task mode: path argument required
			if len(args) == 0 {
				return fmt.Errorf("either a task path or --agent is required")
			}

			return runTasks(args[0], workspaceRoot, dryRun, parallel)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview generated prompt without executing")
	cmd.Flags().IntVar(&parallel, "parallel", 1, "max parallel task executions")
	cmd.Flags().StringVar(&agentName, "agent", "", "agent type to run (e.g., doc-updater, readme-updater)")
	cmd.Flags().StringVar(&prRef, "pr", "", "PR reference for PR-triggered agents (e.g., partio-io/cli#42)")
	cmd.Flags().StringVar(&contextJSON, "context", "", "JSON context for the agent")

	return cmd
}

// runTasks executes task YAML files using the task-runner pipeline.
func runTasks(target, workspaceRoot string, dryRun bool, parallel int) error {
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

	var failed bool
	if parallel <= 1 {
		for _, t := range tasks {
			if err := executeTask(t, workspaceRoot, dryRun); err != nil {
				slog.Error("task failed", "task", t.ID, "error", err)
				failed = true
			}
		}
	} else {
		failed = runParallel(tasks, workspaceRoot, dryRun, parallel)
	}

	fmt.Println("==========================================")
	fmt.Println("All tasks complete.")
	fmt.Println("==========================================")

	if failed {
		return fmt.Errorf("one or more tasks failed")
	}
	return nil
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

	// Build labels CSV
	labelsCSV := "minion"
	if len(t.PRLabels) > 0 {
		labelsCSV = strings.Join(t.PRLabels, ",")
	}
	labels := strings.Split(labelsCSV, ",")

	def := pipeline.Def{
		Name:            "task-runner",
		TaskID:          t.ID,
		WorkspaceRoot:   workspaceRoot,
		TargetRepos:     t.TargetRepos,
		PromptText:      taskPrompt,
		MaxTurns:        cfg.MaxTurns,
		AllowedTools:    "Edit,Write,Read,Glob,Grep,Bash",
		RunChecks:       true,
		RetryOnFail:     true,
		RetryMaxTurns:   15,
		CreatePR:        true,
		PRLabels:        labels,
		TaskTitle:       t.Title,
		TaskDescription: t.Description,
		TaskWhy:         t.Why,
		DryRun:          dryRun,
		DebugDir:        debugDirForTask(t.ID),
	}

	result, err := pipeline.Execute(def)
	if err != nil {
		return err
	}

	for _, url := range result.PRURLs {
		fmt.Printf("PR: %s\n", url)
	}
	fmt.Printf("\nDONE: %s\n\n", t.ID)
	return nil
}

// runAgent loads an agent definition and executes it.
func runAgent(agentName, prRef, contextJSON, workspaceRoot string, dryRun bool) error {
	agentDef, err := agent.Load(agentName)
	if err != nil {
		return err
	}

	fmt.Println("==========================================")
	fmt.Printf("AGENT: %s\n", agentDef.Name)
	fmt.Printf("DESCRIPTION: %s\n", agentDef.Description)
	fmt.Println("==========================================")

	// Parse context JSON
	vars := make(map[string]string)
	if contextJSON != "" {
		if err := json.Unmarshal([]byte(contextJSON), &vars); err != nil {
			return fmt.Errorf("parsing --context JSON: %w", err)
		}
	}

	// Parse --pr flag
	var prRepo, prNumber, repoShort string
	if prRef != "" {
		parts := strings.SplitN(prRef, "#", 2)
		if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
			return fmt.Errorf("invalid PR reference: %s (expected owner/repo#number)", prRef)
		}
		prRepo = parts[0]
		prNumber = parts[1]
		repoShort = prRepo
		if idx := strings.LastIndex(prRepo, "/"); idx >= 0 {
			repoShort = prRepo[idx+1:]
		}
		vars["PR_REF"] = prRef
		vars["PR_REPO"] = prRepo
		vars["PR_NUMBER"] = prNumber
	}

	// Run context providers
	if len(agentDef.ContextProviders) > 0 {
		fmt.Println("--- Gathering context ---")
		input := appcontext.ProviderInput{
			PRRepo:        prRepo,
			PRNumber:      prNumber,
			WorkspaceRoot: workspaceRoot,
			RepoShort:     repoShort,
			PromptsDir:    filepath.Join(workspaceRoot, "minions", "prompts"),
		}
		if err := appcontext.RunProviders(agentDef.ContextProviders, input, vars); err != nil {
			return err
		}
	}

	// Determine target repos
	targetRepos := agentDef.TargetRepos
	if repoShort != "" && len(targetRepos) == 0 {
		// For agents like readme-updater, the target repo is the PR's repo
		targetRepos = []string{repoShort}
	}

	// Build task ID
	taskID := agentDef.Name
	if prRepo != "" && prNumber != "" {
		taskID = fmt.Sprintf("%s-%s-%s", agentDef.Name, strings.ReplaceAll(prRepo, "/", "-"), prNumber)
	}

	// Build PR metadata
	var commitMsg, prTitle, prBody, prRepoFull string
	sourcePRTitle := vars["PR_TITLE"]
	if sourcePRTitle == "" {
		sourcePRTitle = "Unknown"
	}

	if len(targetRepos) == 1 && prRepo != "" {
		prRepoFull = prRepo
		if agentDef.Name == "doc-updater" {
			prRepoFull = "partio-io/docs"
		}

		commitMsg = fmt.Sprintf("docs: update for %s#%s\n\nAutomated by partio-io/minions (%s).\nSource PR: %s#%s\n\nCo-Authored-By: Claude <noreply@anthropic.com>",
			prRepo, prNumber, agentDef.Name, prRepo, prNumber)

		prTitle = fmt.Sprintf("[%s] Update for %s#%s: %s", agentDef.Name, prRepo, prNumber, sourcePRTitle)

		prBody = fmt.Sprintf("## Summary\n\nAutomated update for %s#%s.\n\n**Source PR:** https://github.com/%s/pull/%s\n\n---\n\n*This PR was created by the %s. Please review carefully.*",
			prRepo, prNumber, prRepo, prNumber, agentDef.Name)
	}

	opts := agent.PipelineOpts{
		TaskID:         taskID,
		WorkspaceRoot:  workspaceRoot,
		TargetRepos:    targetRepos,
		CommitMsg:      commitMsg,
		PRTitle:        prTitle,
		PRBody:         prBody,
		PRRepo:         prRepoFull,
		SourcePRRepo:   prRepo,
		SourcePRNumber: prNumber,
		DryRun:         dryRun,
		DebugDir:       debugDirForTask(taskID),
	}

	if cfg.MaxTurns > 0 && cfg.MaxTurns < agentDef.MaxTurns {
		opts.MaxTurns = cfg.MaxTurns
	}

	def, err := agent.BuildPipelineDef(agentDef, vars, opts)
	if err != nil {
		return err
	}

	result, err := pipeline.Execute(*def)
	if err != nil {
		return err
	}

	for _, url := range result.PRURLs {
		fmt.Printf("PR: %s\n", url)
	}
	fmt.Printf("\nDONE: %s\n\n", agentDef.Name)
	return nil
}

func debugDirForTask(taskID string) string {
	base := os.Getenv("MINION_DEBUG_DIR")
	if base == "" {
		return ""
	}
	dir := filepath.Join(base, taskID)
	os.MkdirAll(dir, 0755)
	return dir
}

func runParallel(tasks []*task.Task, workspaceRoot string, dryRun bool, maxParallel int) bool {
	sem := make(chan struct{}, maxParallel)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failed bool

	for _, t := range tasks {
		wg.Add(1)
		sem <- struct{}{}

		go func(t *task.Task) {
			defer wg.Done()
			defer func() { <-sem }()

			if err := executeTask(t, workspaceRoot, dryRun); err != nil {
				slog.Error("task failed", "task", t.ID, "error", err)
				mu.Lock()
				failed = true
				mu.Unlock()
			}
		}(t)
	}

	wg.Wait()
	return failed
}
