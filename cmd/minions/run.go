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

	cmd := &cobra.Command{
		Use:   "run <program.md>",
		Short: "Execute a program",
		Long: `Execute an .md program file, optionally with a GitHub issue as context.

Examples:
  minions run .minions/programs/implement.md --issue 120
  minions run .minions/programs/implement.md --issue partio-io/cli#120
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
			var issueContext string
			if issueRef != "" {
				var err error
				issueContext, err = fetchIssue(issueRef)
				if err != nil {
					return fmt.Errorf("fetching issue: %w", err)
				}
			}

			return runProgram(ctx, args[0], workspaceRoot, issueContext, dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview generated prompt without executing")
	cmd.Flags().StringVar(&issueRef, "issue", "", "GitHub issue number or reference (e.g., 120 or org/repo#120)")

	return cmd
}

// fetchIssue fetches an issue's title and body via gh CLI.
// Accepts either a bare number (uses principal repo) or a full reference (org/repo#123).
func fetchIssue(ref string) (string, error) {
	var repo, number string

	if strings.Contains(ref, "#") {
		parts := strings.SplitN(ref, "#", 2)
		repo = parts[0]
		number = parts[1]
	} else {
		// Bare number — use principal repo from project config
		if proj == nil {
			return "", fmt.Errorf("--issue with bare number requires project config (pass org/repo#number or ensure .minions/project.yaml exists)")
		}
		repo = proj.PrincipalFullName()
		number = ref
	}

	cmd := exec.Command("gh", "issue", "view", number, "--repo", repo, "--json", "title,body", "--jq", `"# " + .title + "\n\n" + .body`)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("gh issue view %s --repo %s: %w", number, repo, err)
	}
	return strings.TrimSpace(string(out)), nil
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

func runProgram(ctx context.Context, programPath, workspaceRoot, issueContext string, dryRun bool) error {
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
