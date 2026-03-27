package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/partio-io/minions/internal/agent"
	appcontext "github.com/partio-io/minions/internal/context"
	"github.com/partio-io/minions/internal/executor"
	"github.com/partio-io/minions/internal/pipeline"
	"github.com/partio-io/minions/internal/planner"
	"github.com/partio-io/minions/internal/pr"
	"github.com/partio-io/minions/internal/program"
	"github.com/partio-io/minions/internal/workspace"
)

func newRunCmd() *cobra.Command {
	var dryRun bool
	var agentName string
	var prRef string
	var contextJSON string
	var programFile string

	cmd := &cobra.Command{
		Use:   "run [program.md]",
		Short: "Execute a program or agent type end-to-end",
		Long: `Execute an .md program file or a named agent type.

Examples:
  minions run programs/detect-hooks.md
  minions run --program programs/detect-hooks.md
  minions run --agent doc-updater --pr my-org/my-repo#42`,
		Args: cobra.MaximumNArgs(1),
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

			// Agent mode: --agent flag specified
			if agentName != "" {
				return runAgent(ctx, agentName, prRef, contextJSON, workspaceRoot, dryRun)
			}

			// Program mode: --program flag or positional arg
			progPath := programFile
			if progPath == "" && len(args) > 0 {
				progPath = args[0]
			}
			if progPath == "" {
				return fmt.Errorf("a program .md file path or --agent is required")
			}

			return runProgram(ctx, progPath, workspaceRoot, dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview generated prompt without executing")
	cmd.Flags().StringVar(&agentName, "agent", "", "agent type to run (e.g., doc-updater, readme-updater)")
	cmd.Flags().StringVar(&prRef, "pr", "", "PR reference for PR-triggered agents (e.g., my-org/my-repo#42)")
	cmd.Flags().StringVar(&contextJSON, "context", "", "JSON context for the agent")
	cmd.Flags().StringVar(&programFile, "program", "", "path to an .md program file")

	return cmd
}

// runAgent loads an agent definition and executes it.
func runAgent(ctx context.Context, agentName, prRef, contextJSON, workspaceRoot string, dryRun bool) error {
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
		if agentDef.Name == "doc-updater" && proj != nil {
			if dr := proj.DocsRepo(); dr != nil {
				prRepoFull = dr.FullName
			}
		}

		principalName := ""
		if proj != nil {
			principalName = proj.PrincipalFullName()
		}

		commitMsg = fmt.Sprintf("docs: update for %s#%s\n\nAutomated by %s (%s).\nSource PR: %s#%s\n\nCo-Authored-By: Claude <noreply@anthropic.com>",
			prRepo, prNumber, principalName, agentDef.Name, prRepo, prNumber)

		prTitle = fmt.Sprintf("[%s] Update for %s#%s: %s", agentDef.Name, prRepo, prNumber, sourcePRTitle)

		prBody = fmt.Sprintf("## Summary\n\nAutomated update for %s#%s.\n\n**Source PR:** https://github.com/%s/pull/%s\n\n---\n\n*This PR was created by the %s. Please review carefully.*",
			prRepo, prNumber, prRepo, prNumber, agentDef.Name)
	}

	// Resolve project config for agent pipeline
	var agentFullNameFn pr.FullNameFunc
	var agentPrincipalRepo string
	if proj != nil {
		agentFullNameFn = pr.FullNameFunc(proj.FullName)
		agentPrincipalRepo = proj.PrincipalFullName()
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
		FullNameFn:     agentFullNameFn,
		PrincipalRepo:  agentPrincipalRepo,
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

	result, err := pipeline.Execute(ctx, *def)
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

// runProgram parses and executes an .md program file.
func runProgram(ctx context.Context, programPath, workspaceRoot string, dryRun bool) error {
	// 1. Parse program
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
	fmt.Printf("DRY RUN: %v\n", dryRun)
	fmt.Println("==========================================")

	// 2. Ensure repos are available
	if proj == nil {
		return fmt.Errorf("project config required: ensure .minions/project.yaml exists in the workspace")
	}
	allRepos := prog.AllTargetRepos()
	if err := workspace.EnsureRepos(proj, workspaceRoot, allRepos); err != nil {
		return fmt.Errorf("ensuring repos: %w", err)
	}

	// 3. Create context tracker
	tracker := appcontext.NewTracker(prog.ID)

	// 4. Run planner (if defined)
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

	// 5. Run executor
	fmt.Println("\n--- Execution Phase ---")
	result, err := executor.Run(ctx, executor.Opts{
		Program:       prog,
		PlanText:      planText,
		WorkspaceRoot: workspaceRoot,
		Project:       proj,
		Tracker:       tracker,
		DryRun:        dryRun,
		DebugDir:      debugDirForTask(prog.ID),
	})
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	// 6. Print results
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

	// 7. Print context report
	report := tracker.Report()
	report.PrintSummary()

	// Write JSON report to debug dir
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

