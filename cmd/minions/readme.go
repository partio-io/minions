package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/partio-io/minions/internal/pipeline"
	"github.com/partio-io/minions/internal/prompt"
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

			// Resolve prompts directory
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

			taskID := fmt.Sprintf("readme-update-%s-%s", strings.ReplaceAll(prRepo, "/", "-"), prNumber)

			sourcePRTitle := prompt.GHPRField(prRepo, prNumber, "title")

			def := pipeline.Def{
				Name:           "readme-updater",
				TaskID:         taskID,
				WorkspaceRoot:  workspaceRoot,
				TargetRepos:    []string{repoShort},
				PromptText:     readmePrompt,
				MaxTurns:       10,
				AllowedTools:   "Edit,Write,Read,Glob,Grep,Bash",
				SkipMarker:     ".no-update-needed",
				RunChecks:      false,
				CreatePR:       true,
				PRLabels:       []string{"minion"},
				PRRepo:         prRepo,
				StageFiles:     []string{"README.md"},
				CommitMsg:      fmt.Sprintf("docs: update README for %s#%s\n\nAutomated README update by partio-io/minions readme-minion.\nSource PR: %s#%s — %s\n\nCo-Authored-By: Claude <noreply@anthropic.com>", prRepo, prNumber, prRepo, prNumber, sourcePRTitle),
				PRTitle:        fmt.Sprintf("[readme] Update for %s#%s: %s", prRepo, prNumber, sourcePRTitle),
				PRBody:         fmt.Sprintf("## Summary\n\nAutomated README update for %s#%s.\n\n**Source PR:** https://github.com/%s/pull/%s\n\n---\n\n*This PR was created by the readme-minion. Please review carefully.*", prRepo, prNumber, prRepo, prNumber),
				SourcePRRepo:   prRepo,
				SourcePRNumber: prNumber,
				DryRun:         dryRun,
			}

			result, err := pipeline.Execute(def)
			if err != nil {
				return err
			}

			if result.Skipped {
				return nil
			}

			for _, url := range result.PRURLs {
				fmt.Printf("README PR: %s\n", url)
			}
			fmt.Printf("\nDONE: README updated for %s\n", prRef)
			return nil
		},
	}

	cmd.Flags().StringVar(&prRef, "pr", "", "PR reference (e.g., partio-io/app#42)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview generated prompt without executing")
	cmd.Flags().StringVar(&promptsDir, "prompts-dir", "prompts", "directory containing per-repo prompt files")

	return cmd
}
