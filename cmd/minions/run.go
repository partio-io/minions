package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	appcontext "github.com/partio-io/minions/internal/context"
	"github.com/partio-io/minions/internal/executor"
	"github.com/partio-io/minions/internal/planner"
	"github.com/partio-io/minions/internal/program"
	"github.com/partio-io/minions/internal/workspace"
)

func newRunCmd() *cobra.Command {
	var dryRun bool
	var issueRef string
	var prRef string

	cmd := &cobra.Command{
		Use:   "run <program.md>",
		Short: "Execute a program",
		Long: `Execute an .md program file, optionally with a GitHub issue or pull request as context.

Examples:
  minions run .minions/programs/implement.md --issue 120
  minions run .minions/programs/implement.md --issue partio-io/cli#120
  minions run .minions/programs/doc-update.md --pr partio-io/cli#488
  minions run .minions/programs/propose.md
  minions run .minions/programs/implement.md --issue 120 --dry-run`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

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

			// Fetch issue context if --issue is provided
			var issueContext, issueNumber string
			if issueRef != "" {
				var err error
				issueContext, issueNumber, err = fetchIssue(issueRef)
				if err != nil {
					return fmt.Errorf("fetching issue: %w", err)
				}
			}

			// Fetch PR context if --pr is provided (mutually exclusive with --issue)
			var prContext, prNumber string
			if prRef != "" {
				var err error
				prContext, prNumber, err = fetchPR(prRef)
				if err != nil {
					return fmt.Errorf("fetching PR: %w", err)
				}
			}

			return runProgram(ctx, args[0], workspaceRoot, issueContext, issueNumber, prContext, prNumber, dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview generated prompt without executing")
	cmd.Flags().StringVar(&issueRef, "issue", "", "GitHub issue number or reference (e.g., 120 or org/repo#120)")
	cmd.Flags().StringVar(&prRef, "pr", "", "GitHub PR number or reference (e.g., 488 or org/repo#488); injects the PR title, body, and diff as context")
	cmd.MarkFlagsMutuallyExclusive("issue", "pr")

	return cmd
}

// parseRef splits a GitHub reference into repo and number. It accepts either a
// full reference (org/repo#123) or a bare number, in which case the principal
// repo from the project config is used. flagName only shapes the error message.
func parseRef(ref, flagName string) (repo, number string, err error) {
	if strings.Contains(ref, "#") {
		parts := strings.SplitN(ref, "#", 2)
		return parts[0], parts[1], nil
	}
	// Bare number — use principal repo from project config
	if proj == nil {
		return "", "", fmt.Errorf("--%s with a bare number requires project config (pass org/repo#number or ensure .minions/project.yaml exists)", flagName)
	}
	return proj.PrincipalFullName(), ref, nil
}

// fetchIssue fetches an issue's title and body via gh CLI and returns the
// parsed issue number alongside the body. Accepts either a bare number (uses
// principal repo) or a full reference (org/repo#123). The number is returned
// so the caller can make the per-build taskID/branch unique.
func fetchIssue(ref string) (string, string, error) {
	repo, number, err := parseRef(ref, "issue")
	if err != nil {
		return "", "", err
	}

	cmd := exec.Command("gh", "issue", "view", number, "--repo", repo, "--json", "title,body", "--jq", `"# " + .title + "\n\n" + .body`)
	out, err := cmd.Output()
	if err != nil {
		return "", "", fmt.Errorf("gh issue view %s --repo %s: %w", number, repo, err)
	}
	return strings.TrimSpace(string(out)), number, nil
}

// fetchPR fetches a pull request's title, body, and full diff via gh CLI and
// returns a context block alongside the parsed PR number. Accepts either a bare
// number (uses principal repo) or a full reference (org/repo#123). The diff is
// embedded so a program can document the change deterministically without a
// further network call, and the leading reference line tells the agent exactly
// which PR — and which repo — the change came from.
func fetchPR(ref string) (string, string, error) {
	repo, number, err := parseRef(ref, "pr")
	if err != nil {
		return "", "", err
	}

	meta, err := exec.Command("gh", "pr", "view", number, "--repo", repo, "--json", "title,body", "--jq", `"# " + .title + "\n\n" + .body`).Output()
	if err != nil {
		return "", "", fmt.Errorf("gh pr view %s --repo %s: %w", number, repo, err)
	}

	diff, err := exec.Command("gh", "pr", "diff", number, "--repo", repo).Output()
	if err != nil {
		return "", "", fmt.Errorf("gh pr diff %s --repo %s: %w", number, repo, err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Source pull request: %s#%s\n\n", repo, number)
	b.WriteString(strings.TrimSpace(string(meta)))
	b.WriteString("\n\n## Diff\n\n```diff\n")
	b.WriteString(strings.TrimSpace(string(diff)))
	b.WriteString("\n```")
	return b.String(), number, nil
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

func runProgram(ctx context.Context, programPath, workspaceRoot, issueContext, issueRef, prContext, prRef string, dryRun bool) error {
	prog, err := program.LoadFile(programPath)
	if err != nil {
		return err
	}
	if err := prog.Validate(); err != nil {
		return fmt.Errorf("invalid program: %w", err)
	}

	fmt.Println("==========================================")
	fmt.Printf("PROGRAM: %s\n", prog.ID)
	fmt.Printf("TITLE: %s\n", prog.Title)
	fmt.Printf("TARGET REPOS: %v\n", prog.AllTargetRepos())
	fmt.Printf("AGENTS: %d\n", len(prog.Agents))
	if issueContext != "" {
		fmt.Println("ISSUE: provided")
	}
	if prContext != "" {
		fmt.Println("PR: provided")
	}
	fmt.Printf("DRY RUN: %v\n", dryRun)
	fmt.Println("==========================================")

	if proj == nil {
		return fmt.Errorf("project config required: ensure .minions/project.yaml exists in the workspace")
	}

	allRepos := prog.AllTargetRepos()
	if err := workspace.EnsureRepos(proj, workspaceRoot, allRepos); err != nil {
		return fmt.Errorf("ensuring repos: %w", err)
	}

	tracker := appcontext.NewTracker(prog.ID)

	var planText string
	if prog.Planner != nil {
		fmt.Println("\n--- Planning Phase ---")
		planResult, err := planner.Run(ctx, planner.Opts{
			Program:       prog,
			WorkspaceRoot: workspaceRoot,
			Project:       proj,
			Tracker:       tracker,
			DryRun:        dryRun,
			DebugDir:      debugDirForTask(prog.ID),
		})
		if err != nil {
			return fmt.Errorf("planning failed: %w", err)
		}
		planText = planResult.Plan
		if planResult.PlanPath != "" {
			fmt.Printf("Plan saved to: %s\n", planResult.PlanPath)
		}
		if planResult.Questions != "" {
			fmt.Printf("\nPlanner questions:\n%s\n", planResult.Questions)
		}
	}

	if dryRun {
		report := tracker.Report()
		report.PrintSummary()
		return nil
	}

	fmt.Println("\n--- Execution Phase ---")
	result, err := executor.Run(ctx, executor.Opts{
		Program:       prog,
		PlanText:      planText,
		IssueContext:  issueContext,
		IssueRef:      issueRef,
		PRContext:     prContext,
		PRRef:         prRef,
		WorkspaceRoot: workspaceRoot,
		Project:       proj,
		Tracker:       tracker,
		DryRun:        dryRun,
		DebugDir:      debugDirForTask(prog.ID),
	})
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	fmt.Println("\n==========================================")
	fmt.Printf("PROGRAM COMPLETE: %s\n", prog.ID)
	var allPRs []string
	var anyFailed bool
	for _, ar := range result.AgentResults {
		if ar.Error != nil {
			fmt.Printf("  FAILED: %s — %v\n", ar.AgentName, ar.Error)
			anyFailed = true
		} else if ar.Skipped {
			fmt.Printf("  SKIPPED: %s — %s\n", ar.AgentName, ar.SkipReason)
		} else {
			for _, url := range ar.PRURLs {
				fmt.Printf("  PR: %s (%s)\n", url, ar.AgentName)
				allPRs = append(allPRs, url)
			}
		}
	}
	fmt.Println("==========================================")

	report := tracker.Report()
	report.PrintSummary()

	if debugDir := debugDirForTask(prog.ID); debugDir != "" {
		_ = report.WriteJSON(filepath.Join(debugDir, "context-report.json"))
	}

	if anyFailed {
		return fmt.Errorf("one or more agents failed")
	}
	if len(allPRs) == 0 {
		slog.Warn("no PRs created by any agent")
	}
	return nil
}
