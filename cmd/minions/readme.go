package main

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/partio-io/minions/internal/claude"
	"github.com/partio-io/minions/internal/git"
	"github.com/partio-io/minions/internal/pr"
	"github.com/partio-io/minions/internal/prompt"
	"github.com/partio-io/minions/internal/worktree"
)

func newReadmeCmd() *cobra.Command {
	var prRef string
	var dryRun bool
	var promptsDir string

	cmd := &cobra.Command{
		Use:   "readme",
		Short: "Update a repo's README based on a merged PR",
		Long:  `Automatically updates README.md based on changes in a merged pull request.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if prRef == "" {
				return fmt.Errorf("--pr is required (e.g., --pr partio-io/app#42)")
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

			// Extract short repo name (e.g., "app" from "partio-io/app")
			repoShort := prRepo
			if idx := strings.LastIndex(prRepo, "/"); idx >= 0 {
				repoShort = prRepo[idx+1:]
			}

			workspaceRoot := cfg.WorkspaceRoot
			if workspaceRoot == "" {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("getting working directory: %w", err)
				}
				workspaceRoot = filepath.Dir(wd)
			}

			// Resolve prompts directory (relative to minions repo)
			if !filepath.IsAbs(promptsDir) {
				minionsDir := filepath.Join(workspaceRoot, "minions")
				promptsDir = filepath.Join(minionsDir, promptsDir)
			}

			fmt.Println("==========================================")
			fmt.Println("README MINION")
			fmt.Printf("PR: %s#%s\n", prRepo, prNumber)
			fmt.Println("==========================================")

			// Build readme prompt
			fmt.Println("--- Building readme prompt ---")
			readmePrompt, err := prompt.BuildReadme(prRepo, prNumber, workspaceRoot, promptsDir)
			if err != nil {
				return fmt.Errorf("building readme prompt: %w", err)
			}

			if dryRun {
				fmt.Println()
				fmt.Println("=== DRY RUN: Generated Prompt ===")
				fmt.Println(readmePrompt)
				fmt.Println("=== END PROMPT ===")
				return nil
			}

			// Create worktree in the same repo the PR was merged to
			targetRepo := filepath.Join(workspaceRoot, repoShort)
			taskID := fmt.Sprintf("readme-update-%s-%s", strings.ReplaceAll(prRepo, "/", "-"), prNumber)

			if _, err := os.Stat(targetRepo); os.IsNotExist(err) {
				return fmt.Errorf("target repo not found at %s", targetRepo)
			}

			fmt.Println("--- Creating worktree ---")
			wtPath, err := worktree.Create(targetRepo, taskID)
			if err != nil {
				return fmt.Errorf("creating worktree: %w", err)
			}
			fmt.Printf("Worktree: %s\n", wtPath)

			// Run Claude Code with max 10 turns (README is a small task)
			fmt.Println("--- Running Claude Code ---")
			maxTurns := 10
			err = claude.Run(claude.Opts{
				Prompt:   readmePrompt,
				CWD:      wtPath,
				MaxTurns: maxTurns,
			})
			if err != nil {
				slog.Warn("claude exited with error", "error", err)
			}

			// Check for .no-update-needed
			noUpdatePath := filepath.Join(wtPath, ".no-update-needed")
			if data, err := os.ReadFile(noUpdatePath); err == nil {
				fmt.Printf("Claude determined no README update is needed:\n%s\n", string(data))
				worktree.Cleanup(targetRepo, taskID)
				return nil
			}

			// Check for actual changes
			status, _ := git.ExecGitDir(wtPath, "status", "--porcelain")
			if strings.TrimSpace(status) == "" {
				fmt.Println("No changes made to README.")
				worktree.Cleanup(targetRepo, taskID)
				return nil
			}

			// Create README PR
			fmt.Println("--- Creating README PR ---")
			branchName := "minion/" + taskID

			if _, err := git.ExecGitDir(wtPath, "add", "README.md"); err != nil {
				return fmt.Errorf("staging changes: %w", err)
			}

			sourcePRTitle := prompt.GHPRField(prRepo, prNumber, "title")
			commitMsg := fmt.Sprintf("docs: update README for %s#%s\n\nAutomated README update by partio-io/minions readme-minion.\nSource PR: %s#%s — %s\n\nCo-Authored-By: Claude <noreply@anthropic.com>", prRepo, prNumber, prRepo, prNumber, sourcePRTitle)
			if _, err := git.ExecGitDir(wtPath, "commit", "-m", commitMsg); err != nil {
				return fmt.Errorf("committing: %w", err)
			}

			if _, err := git.ExecGitDir(wtPath, "push", "-u", "origin", branchName); err != nil {
				return fmt.Errorf("pushing: %w", err)
			}

			// Create PR with minion label (critical for infinite loop prevention)
			prTitle := fmt.Sprintf("[readme] Update for %s#%s: %s", prRepo, prNumber, sourcePRTitle)
			prBody := fmt.Sprintf(`## Summary

Automated README update for %s#%s.

**Source PR:** https://github.com/%s/pull/%s

---

*This PR was created by the readme-minion. Please review carefully.*`, prRepo, prNumber, prRepo, prNumber)

			ghArgs := []string{
				"pr", "create",
				"--repo", prRepo,
				"--head", branchName,
				"--title", prTitle,
				"--body", prBody,
				"--label", "minion",
			}

			cmd2 := exec.Command("gh", ghArgs...)
			out, err := cmd2.CombinedOutput()
			if err != nil {
				return fmt.Errorf("creating PR: %s: %w", strings.TrimSpace(string(out)), err)
			}
			readmePRURL := strings.TrimSpace(string(out))

			fmt.Printf("README PR created: %s\n", readmePRURL)

			// Cross-link back to the source PR
			fmt.Println("--- Cross-linking PRs ---")
			pr.CommentOnPR(prRepo, prNumber, fmt.Sprintf("README update PR: %s", readmePRURL))

			// Cleanup
			fmt.Println("--- Cleaning up ---")
			worktree.Cleanup(targetRepo, taskID)

			fmt.Printf("\nDONE: README updated for %s\n", prRef)
			fmt.Printf("README PR: %s\n", readmePRURL)
			return nil
		},
	}

	cmd.Flags().StringVar(&prRef, "pr", "", "PR reference (e.g., partio-io/app#42)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview generated prompt without executing")
	cmd.Flags().StringVar(&promptsDir, "prompts-dir", "prompts", "directory containing per-repo prompt files")

	return cmd
}
