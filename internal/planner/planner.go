package planner

import (
	gocontext "context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	claudesdk "github.com/partio-io/claude-agent-sdk-go"

	"github.com/partio-io/minions/internal/claude"
	pcontext "github.com/partio-io/minions/internal/context"
	"github.com/partio-io/minions/internal/program"
	"github.com/partio-io/minions/internal/project"
	"github.com/partio-io/minions/internal/worktree"
)

// Opts configures a planning run.
type Opts struct {
	Program       *program.Program
	WorkspaceRoot string
	Project       *project.Project
	Tracker       *pcontext.Tracker
	DryRun        bool
	DebugDir      string
}

// Result holds the output of a planning phase.
type Result struct {
	Plan      string
	Questions string
	PlanPath  string // path to the saved plan file
}

// Run executes the planning phase for a program.
func Run(ctx gocontext.Context, opts Opts) (*Result, error) {
	prog := opts.Program
	planner := prog.Planner

	if planner == nil {
		return nil, fmt.Errorf("program has no planner section")
	}

	maxTurns := planner.MaxTurns
	if maxTurns == 0 {
		maxTurns = 15
	}

	// Start context tracking
	pt := opts.Tracker.StartPhase("planner")

	// Build prompt
	promptText := buildPrompt(prog, opts.WorkspaceRoot, opts.Project, pt)

	if opts.DryRun {
		fmt.Println()
		fmt.Println("=== DRY RUN: Planner Prompt ===")
		fmt.Println(promptText)
		fmt.Println("=== END PLANNER PROMPT ===")
		pt.Finish(nil)
		return &Result{}, nil
	}

	// Create worktrees for target repos
	var worktreePaths []string
	var worktreeRepos []string
	for _, repo := range prog.TargetRepos {
		repoPath := filepath.Join(opts.WorkspaceRoot, repo)
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			slog.Warn("repo not found, skipping", "path", repoPath)
			continue
		}
		taskID := "plan-" + prog.ID
		wtPath, err := worktree.Create(repoPath, taskID)
		if err != nil {
			return nil, fmt.Errorf("creating worktree for %s: %w", repo, err)
		}
		worktreePaths = append(worktreePaths, wtPath)
		worktreeRepos = append(worktreeRepos, repo)
	}

	if len(worktreePaths) == 0 {
		return nil, fmt.Errorf("no worktrees created")
	}

	cleanup := func() {
		for _, repo := range prog.TargetRepos {
			worktree.Cleanup(filepath.Join(opts.WorkspaceRoot, repo), "plan-"+prog.ID)
		}
	}
	defer cleanup()

	// Determine CWD
	claudeCWD, tmpDir, err := buildCWD(worktreePaths, worktreeRepos)
	if err != nil {
		return nil, err
	}
	if tmpDir != "" {
		defer os.RemoveAll(tmpDir)
	}

	// Save debug prompt
	if opts.DebugDir != "" {
		os.MkdirAll(opts.DebugDir, 0755)
		_ = os.WriteFile(filepath.Join(opts.DebugDir, "planner-prompt.md"), []byte(promptText), 0644)
	}

	// Build tools list
	tools := planner.Tools
	if len(tools) == 0 {
		tools = []string{"Read", "Glob", "Grep", "Bash"}
	}

	permMode := planner.PermissionMode
	if permMode == "" {
		permMode = "plan"
	}

	// Build MCP servers map
	mcpServers := buildMCPServers(planner.MCPs)

	slog.Info("running planner", "program", prog.ID, "cwd", claudeCWD, "max_turns", maxTurns)

	var logFile string
	if opts.DebugDir != "" {
		logFile = filepath.Join(opts.DebugDir, "planner-output.json")
	}

	result, err := claude.Run(ctx, claude.Opts{
		Prompt:         promptText,
		CWD:            claudeCWD,
		MaxTurns:       maxTurns,
		AllowedTools:   strings.Join(tools, ","),
		PermissionMode: permMode,
		MaxBudgetUSD:   planner.MaxBudgetUSD,
		MCPServers:     mcpServers,
		LogFile:        logFile,
	})
	if err != nil {
		pt.Finish(nil)
		return nil, fmt.Errorf("planner failed: %w", err)
	}

	// Record metrics
	pt.Finish(&pcontext.InvocationMetrics{
		InputTokens:              result.InputTokens,
		OutputTokens:             result.OutputTokens,
		CacheCreationInputTokens: result.CacheCreationInputTokens,
		CacheReadInputTokens:     result.CacheReadInputTokens,
		NumTurns:                 result.NumTurns,
		DurationMs:               result.DurationMs,
		DurationAPIMs:            result.DurationAPIMs,
		CostUSD:                  result.TotalCostUSD,
	})

	slog.Info("planner completed",
		"program", prog.ID,
		"turns", result.NumTurns,
		"cost_usd", result.TotalCostUSD,
	)

	// Parse plan and questions
	planText, questions := parsePlanResult(result.ResultText)

	// Save plan to file
	planPath, err := savePlan(prog.ID, planText)
	if err != nil {
		slog.Warn("failed to save plan file", "error", err)
	}

	return &Result{
		Plan:      planText,
		Questions: questions,
		PlanPath:  planPath,
	}, nil
}

// buildCWD determines the working directory for Claude.
func buildCWD(worktreePaths, worktreeRepos []string) (cwd, tmpDir string, err error) {
	if len(worktreePaths) > 1 {
		virtualWS, err := os.MkdirTemp("", "minion-plan-workspace-*")
		if err != nil {
			return "", "", fmt.Errorf("creating virtual workspace: %w", err)
		}
		for i, repo := range worktreeRepos {
			if err := os.Symlink(worktreePaths[i], filepath.Join(virtualWS, repo)); err != nil {
				os.RemoveAll(virtualWS)
				return "", "", fmt.Errorf("creating symlink for %s: %w", repo, err)
			}
		}
		return virtualWS, virtualWS, nil
	}
	return worktreePaths[0], "", nil
}

// buildMCPServers converts program MCP definitions to SDK config.
func buildMCPServers(mcps []program.MCPDef) map[string]claudesdk.MCPServerConfig {
	if len(mcps) == 0 {
		return nil
	}
	servers := make(map[string]claudesdk.MCPServerConfig, len(mcps))
	for _, mcp := range mcps {
		switch mcp.Type {
		case "stdio":
			servers[mcp.Name] = &claudesdk.MCPStdioServer{
				Command: mcp.Command,
				Args:    mcp.Args,
				Env:     mcp.Env,
			}
		case "sse":
			servers[mcp.Name] = &claudesdk.MCPSSEServer{
				URL:     mcp.URL,
				Headers: mcp.Headers,
			}
		case "http":
			servers[mcp.Name] = &claudesdk.MCPHTTPServer{
				URL:     mcp.URL,
				Headers: mcp.Headers,
			}
		}
	}
	return servers
}

// savePlan writes the plan to .minion-plans/ directory.
func savePlan(programID, planText string) (string, error) {
	dir := ".minion-plans"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("creating plans directory: %w", err)
	}

	timestamp := time.Now().Format("20060102T150405")
	filename := fmt.Sprintf("%s-%s.md", programID, timestamp)
	path := filepath.Join(dir, filename)

	if err := os.WriteFile(path, []byte(planText), 0644); err != nil {
		return "", fmt.Errorf("writing plan file: %w", err)
	}

	slog.Info("plan saved", "path", path)
	return path, nil
}

// parsePlanResult splits the result text into plan and questions sections.
func parsePlanResult(text string) (plan, questions string) {
	questionsMarkers := []string{
		"### Questions",
		"## Questions",
	}
	for _, marker := range questionsMarkers {
		idx := strings.Index(text, marker)
		if idx != -1 {
			plan = strings.TrimSpace(text[:idx])
			questions = strings.TrimSpace(text[idx+len(marker):])
			if strings.Contains(strings.ToLower(questions), "no questions") {
				questions = ""
			}
			return plan, questions
		}
	}
	return strings.TrimSpace(text), ""
}
