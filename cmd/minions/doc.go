package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/partio-io/minions/internal/checks"
	"github.com/partio-io/minions/internal/claude"
	"github.com/partio-io/minions/internal/git"
	"github.com/partio-io/minions/internal/pr"
	"github.com/partio-io/minions/internal/prompt"
	"github.com/partio-io/minions/internal/worktree"
)

func newDocCmd() *cobra.Command {
	var prRef string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "doc",
		Short: "Generate a docs PR for an existing code PR",
		Long:  `Automatically generates a documentation update PR based on changes in a source PR.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if prRef == "" {
				return fmt.Errorf("--pr is required (e.g., --pr partio-io/cli#42)")
			}

			if cfg.DryRun {
				dryRun = true
			}

			parts := strings.SplitN(prRef, "#", 2)
			if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
				return fmt.Errorf("invalid PR reference: %s (expected owner/repo#number)", prRef)
			}
			prRepo := parts[0]
			prNumber := parts[1]

			workspaceRoot := cfg.WorkspaceRoot
			if workspaceRoot == "" {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("getting working directory: %w", err)
				}
				workspaceRoot = filepath.Dir(wd)
			}

			fmt.Println("==========================================")
			fmt.Println("DOC MINION")
			fmt.Printf("PR: %s#%s\n", prRepo, prNumber)
			fmt.Println("==========================================")

			// Build doc prompt
			fmt.Println("--- Building doc prompt ---")
			docPrompt, err := prompt.BuildDoc(prRepo, prNumber, workspaceRoot)
			if err != nil {
				return fmt.Errorf("building doc prompt: %w", err)
			}

			if dryRun {
				fmt.Println()
				fmt.Println("=== DRY RUN: Generated Prompt ===")
				fmt.Println(docPrompt)
				fmt.Println("=== END PROMPT ===")
				return nil
			}

			// Create worktree in docs repo
			docsRepo := filepath.Join(workspaceRoot, "docs")
			taskID := fmt.Sprintf("doc-update-%s-%s", strings.ReplaceAll(prRepo, "/", "-"), prNumber)

			if _, err := os.Stat(docsRepo); os.IsNotExist(err) {
				return fmt.Errorf("docs repo not found at %s", docsRepo)
			}

			fmt.Println("--- Creating worktree ---")
			wtPath, err := worktree.Create(docsRepo, taskID)
			if err != nil {
				return fmt.Errorf("creating worktree: %w", err)
			}
			fmt.Printf("Worktree: %s\n", wtPath)

			// Run Claude Code
			fmt.Println("--- Running Claude Code ---")
			maxTurns := cfg.MaxTurns
			if maxTurns > 20 {
				maxTurns = 20
			}
			err = claude.Run(claude.Opts{
				Prompt:   docPrompt,
				CWD:      wtPath,
				MaxTurns: maxTurns,
			})
			if err != nil {
				slog.Warn("claude exited with error", "error", err)
			}

			// Check for .no-update-needed
			noUpdatePath := filepath.Join(wtPath, ".no-update-needed")
			if data, err := os.ReadFile(noUpdatePath); err == nil {
				fmt.Printf("Claude determined no docs update is needed:\n%s\n", string(data))
				worktree.Cleanup(docsRepo, taskID)
				return nil
			}

			// Run docs checks
			fmt.Println("--- Running docs checks ---")
			output, err := checks.Run(wtPath)
			if err != nil {
				fmt.Println("--- Checks failed, retrying ---")
				retryPrompt := fmt.Sprintf("The following docs checks failed. Please fix:\n\n%s\n\nFix the errors. Do not change anything beyond what's needed to fix the failures.", output)

				_ = claude.Run(claude.Opts{
					Prompt:   retryPrompt,
					CWD:      wtPath,
					MaxTurns: 10,
				})

				if _, err := checks.Run(wtPath); err != nil {
					worktree.Cleanup(docsRepo, taskID)
					return fmt.Errorf("docs checks still failing after retry")
				}
			}

			// Check for actual changes
			status, _ := git.ExecGitDir(wtPath, "status", "--porcelain")
			if strings.TrimSpace(status) == "" {
				fmt.Println("No changes made to docs.")
				worktree.Cleanup(docsRepo, taskID)
				return nil
			}

			// Create docs PR
			fmt.Println("--- Creating docs PR ---")
			branchName := "minion/" + taskID

			if _, err := git.ExecGitDir(wtPath, "add", "-A"); err != nil {
				return fmt.Errorf("staging changes: %w", err)
			}

			commitMsg := fmt.Sprintf("docs: update for %s#%s\n\nAutomated documentation update by partio-io/minions doc-minion.\nSource PR: %s#%s\n\nCo-Authored-By: Claude <noreply@anthropic.com>", prRepo, prNumber, prRepo, prNumber)
			if _, err := git.ExecGitDir(wtPath, "commit", "-m", commitMsg); err != nil {
				return fmt.Errorf("committing: %w", err)
			}

			if _, err := git.ExecGitDir(wtPath, "push", "-u", "origin", branchName); err != nil {
				return fmt.Errorf("pushing: %w", err)
			}

			// Fetch source PR title
			sourcePRTitle := prompt.GHPRField(prRepo, prNumber, "title")

			docsPRURL, err := pr.CreateDocsPR(prRepo, prNumber, branchName, sourcePRTitle)
			if err != nil {
				return fmt.Errorf("creating docs PR: %w", err)
			}

			fmt.Printf("Docs PR created: %s\n", docsPRURL)

			// Cross-link
			fmt.Println("--- Cross-linking PRs ---")
			pr.CommentOnPR(prRepo, prNumber, fmt.Sprintf("Documentation update PR: %s", docsPRURL))

			// Cleanup
			fmt.Println("--- Cleaning up ---")
			worktree.Cleanup(docsRepo, taskID)

			fmt.Printf("\nDONE: Documentation updated for %s\n", prRef)
			fmt.Printf("Docs PR: %s\n", docsPRURL)
			return nil
		},
	}

	cmd.Flags().StringVar(&prRef, "pr", "", "PR reference (e.g., partio-io/cli#42)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview generated prompt without executing")

	return cmd
}
