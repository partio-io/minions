package main

import (
	"context"
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
	"github.com/partio-io/minions/internal/executor"
	"github.com/partio-io/minions/internal/pipeline"
	"github.com/partio-io/minions/internal/planner"
	"github.com/partio-io/minions/internal/pr"
	"github.com/partio-io/minions/internal/program"
	"github.com/partio-io/minions/internal/prompt"
	"github.com/partio-io/minions/internal/task"
	"github.com/partio-io/minions/internal/workspace"
)

func newRunCmd() *cobra.Command {
	var dryRun bool
	var parallel int
	var agentName string
	var prRef string
	var contextJSON string
	var planFile string
	var discover bool
	var programFile string

	cmd := &cobra.Command{
		Use:   "run [path]",
		Short: "Execute task specs, agent types, or programs end-to-end",
		Long: `Execute one or more task YAML files, a named agent type, or an .md program.

Examples:
  minions run tasks/my-task.yaml
  minions run tasks/ --parallel 3
  minions run --agent doc-updater --pr my-org/my-repo#42
  minions run --program programs/detect-hooks.md`,
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

			// Program mode: --program flag specified
			if programFile != "" {
				return runProgram(ctx, programFile, workspaceRoot, dryRun)
			}

			// Agent mode: --agent flag specified
			if agentName != "" {
				return runAgent(ctx, agentName, prRef, contextJSON, workspaceRoot, dryRun)
			}

			// Load plan text if provided
			var planText string
			if planFile != "" {
				data, err := os.ReadFile(planFile)
				if err != nil {
					return fmt.Errorf("reading plan file: %w", err)
				}
				planText = string(data)
			}

			// Discover mode: find tasks from all repos in the project
			if discover {
				if proj == nil {
					return fmt.Errorf("--discover requires a project config (.minions/project.yaml)")
				}
				return runDiscoveredTasks(ctx, workspaceRoot, dryRun, parallel, planText)
			}

			// Task mode: path argument required
			if len(args) == 0 {
				return fmt.Errorf("either a task path, --agent, or --discover is required")
			}

			return runTasks(ctx, args[0], workspaceRoot, dryRun, parallel, planText)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview generated prompt without executing")
	cmd.Flags().IntVar(&parallel, "parallel", 1, "max parallel task executions")
	cmd.Flags().StringVar(&agentName, "agent", "", "agent type to run (e.g., doc-updater, readme-updater)")
	cmd.Flags().StringVar(&prRef, "pr", "", "PR reference for PR-triggered agents (e.g., partio-io/cli#42)")
	cmd.Flags().StringVar(&contextJSON, "context", "", "JSON context for the agent")
	cmd.Flags().StringVar(&planFile, "plan-file", "", "path to a plan file to include as context")
	cmd.Flags().BoolVar(&discover, "discover", false, "discover tasks from .minions/tasks/ in all project repos")
	cmd.Flags().StringVar(&programFile, "program", "", "path to an .md program file")

	return cmd
}

// runTasks executes task YAML files using the task-runner pipeline.
func runTasks(ctx context.Context, target, workspaceRoot string, dryRun bool, parallel int, planText string) error {
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
			if err := executeTask(ctx, t, workspaceRoot, dryRun, planText); err != nil {
				slog.Error("task failed", "task", t.ID, "error", err)
				failed = true
			}
		}
	} else {
		failed = runParallel(ctx, tasks, workspaceRoot, dryRun, parallel, planText)
	}

	fmt.Println("==========================================")
	fmt.Println("All tasks complete.")
	fmt.Println("==========================================")

	if failed {
		return fmt.Errorf("one or more tasks failed")
	}
	return nil
}

func executeTask(ctx context.Context, t *task.Task, workspaceRoot string, dryRun bool, planText string) error {
	fmt.Println("==========================================")
	fmt.Printf("TASK: %s\n", t.ID)
	fmt.Printf("TITLE: %s\n", t.Title)
	fmt.Println("==========================================")

	// Build prompt
	fmt.Println("--- Building prompt ---")
	taskPrompt, err := prompt.BuildTask(t, workspaceRoot, proj)
	if err != nil {
		return fmt.Errorf("building prompt: %w", err)
	}

	// Build labels CSV
	labelsCSV := "minion"
	if len(t.PRLabels) > 0 {
		labelsCSV = strings.Join(t.PRLabels, ",")
	}
	labels := strings.Split(labelsCSV, ",")

	// Resolve repo names and principal repo from project config
	if proj == nil {
		return fmt.Errorf("project config required: ensure .minions/project.yaml exists in the workspace")
	}
	fullNameFn := pr.FullNameFunc(proj.FullName)
	principalRepo := proj.PrincipalFullName()

	def := pipeline.Def{
		Name:            "task-runner",
		TaskID:          t.ID,
		WorkspaceRoot:   workspaceRoot,
		TargetRepos:     t.TargetRepos,
		PromptText:      taskPrompt,
		PlanText:        planText,
		MaxTurns:        cfg.MaxTurns,
		AllowedTools:    "Edit,Write,Read,Glob,Grep,Bash",
		RunChecks:       true,
		RetryOnFail:     true,
		RetryMaxTurns:   15,
		CreatePR:        true,
		PRLabels:        labels,
		TaskTitle:          t.Title,
		TaskDescription:    t.Description,
		TaskWhy:            t.Why,
		TaskSource:         t.Source,
		AcceptanceCriteria: t.AcceptanceCriteria,
		FullNameFn:         fullNameFn,
		PrincipalRepo:      principalRepo,
		DryRun:             dryRun,
		DebugDir:        debugDirForTask(t.ID),
	}

	result, err := pipeline.Execute(ctx, def)
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

// runDiscoveredTasks discovers tasks from .minions/tasks/ across all project repos and executes them.
func runDiscoveredTasks(ctx context.Context, workspaceRoot string, dryRun bool, parallel int, planText string) error {
	tasks, err := task.DiscoverAll(workspaceRoot, proj.RepoNames())
	if err != nil {
		return fmt.Errorf("discovering tasks: %w", err)
	}

	if len(tasks) == 0 {
		fmt.Println("No tasks discovered from project repos.")
		return nil
	}

	fmt.Printf("Discovered %d task(s) from project repos\n", len(tasks))
	fmt.Printf("Workspace root: %s\n", workspaceRoot)
	fmt.Printf("Parallel: %d\n", parallel)
	fmt.Printf("Dry run: %v\n\n", dryRun)

	var failed bool
	if parallel <= 1 {
		for _, t := range tasks {
			if err := executeTask(ctx, t, workspaceRoot, dryRun, planText); err != nil {
				slog.Error("task failed", "task", t.ID, "error", err)
				failed = true
			}
		}
	} else {
		failed = runParallel(ctx, tasks, workspaceRoot, dryRun, parallel, planText)
	}

	fmt.Println("==========================================")
	fmt.Println("All discovered tasks complete.")
	fmt.Println("==========================================")

	if failed {
		return fmt.Errorf("one or more tasks failed")
	}
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

func runParallel(ctx context.Context, tasks []*task.Task, workspaceRoot string, dryRun bool, maxParallel int, planText string) bool {
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

			if err := executeTask(ctx, t, workspaceRoot, dryRun, planText); err != nil {
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
