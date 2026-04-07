package executor

import (
	gocontext "context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	claudesdk "github.com/partio-io/claude-agent-sdk-go"

	"github.com/partio-io/minions/internal/checks"
	"github.com/partio-io/minions/internal/claude"
	pcontext "github.com/partio-io/minions/internal/context"
	"github.com/partio-io/minions/internal/git"
	"github.com/partio-io/minions/internal/pr"
	"github.com/partio-io/minions/internal/program"
	"github.com/partio-io/minions/internal/project"
	"github.com/partio-io/minions/internal/worktree"
)

// Opts configures the execution phase.
type Opts struct {
	Program       *program.Program
	PlanText      string
	IssueContext  string // fetched issue body, injected into prompt
	WorkspaceRoot string
	Project       *project.Project
	Tracker       *pcontext.Tracker
	DryRun        bool
	DebugDir      string
}

// Result holds the outcome of the execution phase.
type Result struct {
	AgentResults []AgentResult
}

// AgentResult holds the outcome for a single sub-agent.
type AgentResult struct {
	AgentName  string
	PRURLs     []string
	Skipped    bool
	SkipReason string
	Error      error
}

// Run executes all sub-agents sequentially.
func Run(ctx gocontext.Context, opts Opts) (*Result, error) {
	prog := opts.Program
	agents := prog.Agents

	// If no agents defined, create an implicit one from the program description
	if len(agents) == 0 {
		agents = []program.AgentDef{{
			Name:         prog.ID,
			Tools:        []string{"Edit", "Write", "Read", "Glob", "Grep", "Bash"},
			MaxTurns:     30,
			Checks:       true,
			RetryOnFail:  true,
			Instructions: prog.Description,
		}}
	}

	result := &Result{}

	for i := range agents {
		agent := &agents[i]
		slog.Info("running agent", "name", agent.Name, "index", i+1, "total", len(agents))

		agentResult := runAgent(ctx, opts, prog, agent)
		result.AgentResults = append(result.AgentResults, agentResult)

		if agentResult.Error != nil {
			slog.Error("agent failed", "name", agent.Name, "error", agentResult.Error)
			// Continue with next agent; don't abort the whole program
		}

		if len(agentResult.PRURLs) > 0 {
			for _, url := range agentResult.PRURLs {
				fmt.Printf("PR created: %s\n", url)
			}
		}
	}

	return result, nil
}

// runAgent executes a single sub-agent.
func runAgent(ctx gocontext.Context, opts Opts, prog *program.Program, agent *program.AgentDef) AgentResult {
	repos := prog.EffectiveTargetRepos(agent)
	taskID := prog.ID + "-" + agent.Name
	multiRepo := len(repos) > 1

	// Start context tracking
	pt := opts.Tracker.StartPhase("agent:" + agent.Name)

	// Build prompt
	promptText := buildAgentPrompt(prog, agent, opts.PlanText, opts.IssueContext, opts.WorkspaceRoot, opts.Project, pt)

	if opts.DryRun {
		fmt.Printf("\n=== DRY RUN: Agent %s Prompt ===\n", agent.Name)
		fmt.Println(promptText)
		fmt.Println("=== END AGENT PROMPT ===")
		pt.Finish(nil)
		return AgentResult{AgentName: agent.Name}
	}

	// Save debug prompt
	if opts.DebugDir != "" {
		os.MkdirAll(opts.DebugDir, 0755)
		_ = os.WriteFile(filepath.Join(opts.DebugDir, "agent-"+agent.Name+"-prompt.md"), []byte(promptText), 0644)
	}

	// Create worktrees
	fmt.Printf("--- Creating worktrees for agent %s ---\n", agent.Name)
	var worktreePaths []string
	var worktreeRepos []string
	for _, repo := range repos {
		repoPath := filepath.Join(opts.WorkspaceRoot, repo)
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			if multiRepo {
				slog.Warn("repo not found, skipping", "path", repoPath)
				continue
			}
			pt.Finish(nil)
			return AgentResult{AgentName: agent.Name, Error: fmt.Errorf("repo not found at %s", repoPath)}
		}
		wtPath, err := worktree.Create(repoPath, taskID)
		if err != nil {
			pt.Finish(nil)
			return AgentResult{AgentName: agent.Name, Error: fmt.Errorf("creating worktree for %s: %w", repo, err)}
		}
		worktreePaths = append(worktreePaths, wtPath)
		worktreeRepos = append(worktreeRepos, repo)
	}

	if len(worktreePaths) == 0 {
		pt.Finish(nil)
		return AgentResult{AgentName: agent.Name, Error: fmt.Errorf("no worktrees created")}
	}

	cleanup := func() {
		for _, repo := range repos {
			worktree.Cleanup(filepath.Join(opts.WorkspaceRoot, repo), taskID)
		}
	}

	// Determine CWD
	claudeCWD, tmpDir, err := buildCWD(worktreePaths, worktreeRepos)
	if err != nil {
		cleanup()
		pt.Finish(nil)
		return AgentResult{AgentName: agent.Name, Error: err}
	}
	if tmpDir != "" {
		defer os.RemoveAll(tmpDir)
	}

	// Tools
	tools := agent.Tools
	if len(tools) == 0 {
		tools = []string{"Edit", "Write", "Read", "Glob", "Grep", "Bash"}
	}

	maxTurns := agent.MaxTurns
	if maxTurns == 0 {
		maxTurns = 30
	}

	// MCP servers
	mcpServers := buildMCPServers(agent.MCPs)

	// Run Claude
	fmt.Printf("--- Running Claude for agent %s ---\n", agent.Name)
	var logFile string
	if opts.DebugDir != "" {
		logFile = filepath.Join(opts.DebugDir, "agent-"+agent.Name+"-output.json")
	}

	result, err := claude.Run(ctx, claude.Opts{
		Prompt:         promptText,
		CWD:            claudeCWD,
		MaxTurns:       maxTurns,
		AllowedTools:   strings.Join(tools, ","),
		PermissionMode: "bypassPermissions",
		MaxBudgetUSD:   agent.MaxBudgetUSD,
		MCPServers:     mcpServers,
		LogFile:        logFile,
	})
	if err != nil {
		cleanup()
		pt.Finish(nil)
		return AgentResult{AgentName: agent.Name, Error: fmt.Errorf("claude failed: %w", err)}
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

	if result.IsError {
		slog.Warn("claude returned error result", "agent", agent.Name, "subtype", result.Subtype)
	}

	// Check skip marker
	if agent.SkipMarker != "" {
		for _, wtPath := range worktreePaths {
			markerPath := filepath.Join(wtPath, agent.SkipMarker)
			if data, err := os.ReadFile(markerPath); err == nil {
				fmt.Printf("Agent %s determined no update is needed: %s\n", agent.Name, strings.TrimSpace(string(data)))
				cleanup()
				return AgentResult{AgentName: agent.Name, Skipped: true, SkipReason: strings.TrimSpace(string(data))}
			}
		}
	}

	// Check for changes — both uncommitted and committed
	hasChanges := false
	for i, wtPath := range worktreePaths {
		// Check uncommitted changes
		status, _ := git.ExecGitDir(wtPath, "status", "--porcelain")
		slog.Info("worktree status", "agent", agent.Name, "repo", worktreeRepos[i], "status", strings.TrimSpace(status))
		if strings.TrimSpace(status) != "" {
			hasChanges = true
			slog.Info("worktree has uncommitted changes", "agent", agent.Name, "repo", worktreeRepos[i])
			continue
		}
		// Check if there are new commits on the worktree branch vs the base
		logOut, _ := git.ExecGitDir(wtPath, "log", "HEAD", "--not", "--remotes", "--oneline")
		slog.Info("worktree log", "agent", agent.Name, "repo", worktreeRepos[i], "log", strings.TrimSpace(logOut))

		// Also check diff against origin/main or origin/HEAD
		diffStat, _ := git.ExecGitDir(wtPath, "diff", "--stat", "origin/HEAD...HEAD")
		slog.Info("worktree diff vs origin", "agent", agent.Name, "repo", worktreeRepos[i], "diff_stat", strings.TrimSpace(diffStat))

		if strings.TrimSpace(logOut) != "" || strings.TrimSpace(diffStat) != "" {
			hasChanges = true
			slog.Info("worktree has new commits", "agent", agent.Name, "repo", worktreeRepos[i])
		}
	}
	if !hasChanges {
		fmt.Printf("Agent %s produced no changes.\n", agent.Name)
		cleanup()
		return AgentResult{AgentName: agent.Name, Skipped: true, SkipReason: "no changes"}
	}

	// Run checks
	if agent.Checks {
		allPass, failedOutput := runChecks(worktreePaths)

		if !allPass && agent.RetryOnFail {
			fmt.Printf("--- Checks failed for agent %s, retrying ---\n", agent.Name)
			retryMaxTurns := agent.RetryMaxTurns
			if retryMaxTurns == 0 {
				retryMaxTurns = 15
			}

			retryPrompt := fmt.Sprintf("The following checks failed after your implementation. Please fix the issues:\n\n%s\n\nFix the errors and ensure all checks pass.", failedOutput)

			var retryLogFile string
			if opts.DebugDir != "" {
				retryLogFile = filepath.Join(opts.DebugDir, "agent-"+agent.Name+"-retry-output.json")
			}

			retryResult, retryErr := claude.Run(ctx, claude.Opts{
				Prompt:       retryPrompt,
				CWD:          claudeCWD,
				MaxTurns:     retryMaxTurns,
				AllowedTools: strings.Join(tools, ","),
				LogFile:      retryLogFile,
			})
			if retryErr != nil {
				slog.Error("claude retry failed", "agent", agent.Name, "error", retryErr)
			} else if retryResult.IsError {
				slog.Warn("claude retry returned error", "agent", agent.Name, "subtype", retryResult.Subtype)
			}

			allPass, _ = runChecks(worktreePaths)
		}

		if !allPass {
			slog.Error("checks still failing", "agent", agent.Name)
			cleanup()
			return AgentResult{AgentName: agent.Name, Error: fmt.Errorf("checks failed for agent %s", agent.Name)}
		}
	}

	// Create PRs
	fmt.Printf("--- Creating PRs for agent %s ---\n", agent.Name)
	labelsCSV := strings.Join(prog.PRLabels, ",")
	if labelsCSV == "" {
		labelsCSV = "minion"
	}

	var fullNameFn pr.FullNameFunc
	var principalRepo string
	if opts.Project != nil {
		fullNameFn = opts.Project.FullName
		principalRepo = opts.Project.PrincipalFullName()
	}

	if fullNameFn == nil || principalRepo == "" {
		cleanup()
		return AgentResult{AgentName: agent.Name, Error: fmt.Errorf("project config required for PR creation")}
	}

	// Summarize changes for PR title and description
	prTitle, prDescription := summarizeChanges(ctx, claudeCWD, prog, opts.IssueContext)

	prOpts := &pr.CreateOpts{
		AcceptanceCriteria: prog.AcceptanceCriteria,
		Source:             prog.Source,
	}
	prURLs, err := pr.CreateAndLinkAll(taskID, prTitle, prDescription, "", opts.WorkspaceRoot, labelsCSV, worktreeRepos, fullNameFn, principalRepo, prOpts)
	if err != nil {
		cleanup()
		return AgentResult{AgentName: agent.Name, Error: fmt.Errorf("PR creation failed: %w", err)}
	}

	// Write PR URLs to file if env var set
	if prURLsFile := os.Getenv("MINION_PR_URLS_FILE"); prURLsFile != "" {
		if f, err := os.OpenFile(prURLsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			for _, u := range prURLs {
				fmt.Fprintln(f, u)
			}
			f.Close()
		}
	}

	cleanup()
	return AgentResult{AgentName: agent.Name, PRURLs: prURLs}
}

// buildCWD determines the working directory for Claude.
func buildCWD(worktreePaths, worktreeRepos []string) (cwd, tmpDir string, err error) {
	if len(worktreePaths) > 1 {
		virtualWS, err := os.MkdirTemp("", "minion-workspace-*")
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

// summarizeChanges runs a cheap Claude call to generate a PR title and description from the diff.
func summarizeChanges(ctx gocontext.Context, cwd string, prog *program.Program, issueContext string) (title, description string) {
	// Get the diff
	diffCmd := exec.Command("git", "diff", "HEAD")
	diffCmd.Dir = cwd
	diffOut, err := diffCmd.Output()
	if err != nil || len(diffOut) == 0 {
		// Fallback: try diff of staged + unstaged
		diffCmd = exec.Command("git", "diff")
		diffCmd.Dir = cwd
		diffOut, _ = diffCmd.Output()
	}

	diff := string(diffOut)
	if len(diff) > 20000 {
		diff = diff[:20000] + "\n... (truncated)"
	}

	var prompt strings.Builder
	prompt.WriteString("You are writing a PR title and description for a code change. Be concise and specific.\n\n")

	if issueContext != "" {
		prompt.WriteString("## Original Issue\n\n")
		prompt.WriteString(issueContext)
		prompt.WriteString("\n\n")
	}

	prompt.WriteString("## Diff\n\n```\n")
	prompt.WriteString(diff)
	prompt.WriteString("\n```\n\n")
	prompt.WriteString("Write a PR title and description. Format your response EXACTLY as:\n\n")
	prompt.WriteString("TITLE: <concise PR title, no prefix like [minion]>\n\n")
	prompt.WriteString("DESCRIPTION:\n<what was implemented, key decisions, how to test>\n")

	fmt.Println("--- Summarizing changes for PR ---")
	result, err := claude.Run(ctx, claude.Opts{
		Prompt:   prompt.String(),
		CWD:      cwd,
		MaxTurns: 5,
	})
	if err != nil || result.ResultText == "" {
		slog.Warn("failed to summarize changes, using program title", "error", err)
		return prog.Title, prog.Description
	}

	return parseSummary(result.ResultText, prog.Title, prog.Description)
}

// parseSummary extracts title and description from the summarize response.
func parseSummary(text, fallbackTitle, fallbackDesc string) (string, string) {
	title := fallbackTitle
	desc := fallbackDesc

	lines := strings.Split(text, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, "TITLE:") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "TITLE:"))
		}
		if strings.HasPrefix(line, "DESCRIPTION:") {
			// Everything after DESCRIPTION: line
			rest := strings.Join(lines[i+1:], "\n")
			desc = strings.TrimSpace(rest)
			break
		}
	}

	return title, desc
}

// runChecks runs deterministic checks on worktree paths.
func runChecks(worktreePaths []string) (bool, string) {
	fmt.Println("--- Running checks ---")
	allPass := true
	var failed strings.Builder
	for _, wtPath := range worktreePaths {
		output, err := checks.Run(wtPath)
		if err != nil {
			allPass = false
			failed.WriteString(output)
			failed.WriteByte('\n')
		}
	}
	return allPass, failed.String()
}
