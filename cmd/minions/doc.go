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

func newDocCmd() *cobra.Command {
	var prRef string
	var dryRun bool

	cmd := &cobra.Command{
		Use:   "doc",
		Short: "Generate a docs PR for an existing code PR",
		Long:  `Automatically generates a documentation update PR based on changes in a source PR.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

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

			taskID := fmt.Sprintf("doc-update-%s-%s", strings.ReplaceAll(prRepo, "/", "-"), prNumber)

			maxTurns := cfg.MaxTurns
			if maxTurns > 20 {
				maxTurns = 20
			}

			sourcePRTitle := prompt.GHPRField(prRepo, prNumber, "title")

			// Resolve docs repo from project config
			docsRepoName := "docs"
			docsRepoFull := ""
			principalName := ""
			if proj != nil {
				if dr := proj.DocsRepo(); dr != nil {
					docsRepoName = dr.Name
					docsRepoFull = dr.FullName
				}
				principalName = proj.PrincipalFullName()
			}
			if docsRepoFull == "" {
				return fmt.Errorf("project config required: no docs repo configured in .minions/project.yaml")
			}

			def := pipeline.Def{
				Name:           "doc-updater",
				TaskID:         taskID,
				WorkspaceRoot:  workspaceRoot,
				TargetRepos:    []string{docsRepoName},
				PromptText:     docPrompt,
				MaxTurns:       maxTurns,
				AllowedTools:   "Edit,Write,Read,Glob,Grep,Bash",
				SkipMarker:     ".no-update-needed",
				RunChecks:      true,
				RetryOnFail:    true,
				RetryMaxTurns:  10,
				CreatePR:       true,
				PRLabels:       []string{"minion", "documentation"},
				PRRepo:         docsRepoFull,
				CommitMsg:      fmt.Sprintf("docs: update for %s#%s\n\nAutomated documentation update by %s doc-minion.\nSource PR: %s#%s\n\nCo-Authored-By: Claude <noreply@anthropic.com>", prRepo, prNumber, principalName, prRepo, prNumber),
				PRTitle:        fmt.Sprintf("[docs] Update for %s#%s: %s", prRepo, prNumber, sourcePRTitle),
				PRBody:         fmt.Sprintf("## Summary\n\nAutomated documentation update for %s#%s.\n\n**Source PR:** https://github.com/%s/pull/%s\n\n---\n\n*This PR was created by the doc-minion. Please review carefully.*", prRepo, prNumber, prRepo, prNumber),
				SourcePRRepo:   prRepo,
				SourcePRNumber: prNumber,
				DryRun:         dryRun,
			}

			result, err := pipeline.Execute(ctx, def)
			if err != nil {
				return err
			}

			if result.Skipped {
				return nil
			}

			for _, url := range result.PRURLs {
				fmt.Printf("Docs PR: %s\n", url)
			}
			fmt.Printf("\nDONE: Documentation updated for %s\n", prRef)
			return nil
		},
	}

	cmd.Flags().StringVar(&prRef, "pr", "", "PR reference (e.g., partio-io/cli#42)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview generated prompt without executing")

	return cmd
}
