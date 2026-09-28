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
	"github.com/partio-io/minions/internal/slices"
	"github.com/partio-io/minions/internal/worktree"
)

// Seams over external collaborators so executor tests can stub Claude
// sessions, checks, and PR creation while real temp git repos exercise the
// worktree and push mechanics.
var (
	claudeRun          = claude.Run
	checksRun          = checks.Run
	prCreateAndLinkAll = pr.CreateAndLinkAll
	prURLForBranch     = ghPRURLForBranch
	postIssueComment   = pr.CommentOnIssue
)

// ghPRURLForBranch returns the URL of the open PR whose head is branch, or ""
// when none exists. It runs gh in repoPath so the repo is resolved from the
// checkout's origin remote.
func ghPRURLForBranch(repoPath, branch string) (string, error) {
	cmd := exec.Command("gh", "pr", "list", "--head", branch, "--json", "url", "--jq", ".[].url")
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr list --head %s: %s: %w", branch, strings.TrimSpace(string(out)), err)
	}
	url, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n")
	return url, nil
}

// Opts configures the execution phase.
type Opts struct {
	Program       *program.Program
	PlanText      string
	IssueContext  string           // fetched issue body, injected into prompt
	IssueTitle    string           // issue title, structured (slice-aware path)
	IssueBody     string           // issue body, structured (slice-aware path)
	IssueComments []slices.Comment // fetched issue comments, structured (slice-plan detection)
	IssueRef      string           // issue number (e.g. "437"); appended to taskID so each build gets its own branch/PR
	PRContext     string           // fetched PR title, body, and diff, injected into prompt
	PRRef         string           // PR number (e.g. "488"); appended to taskID so each build gets its own branch/PR
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

	plan, err := resolveSlicePlan(prog, opts.IssueComments)
	if err != nil {
		return nil, err
	}

	result := &Result{}

	for i := range agents {
		agent := &agents[i]
		slog.Info("running agent", "name", agent.Name, "index", i+1, "total", len(agents))

		agentResult := runAgent(ctx, opts, prog, agent, plan)
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

// buildTaskID derives the per-run task identifier used for the worktree path
// and the PR branch name (minion/<taskID>). When an issue reference is present
// it is appended, so each issue-triggered build gets its own branch and PR
// instead of colliding on a single shared branch shared across every run.
func buildTaskID(progID, agentName, issueRef string) string {
	id := progID + "-" + agentName
	if issueRef != "" {
		id += "-" + issueRef
	}
	return id
}

// runAgent executes a single sub-agent. A non-nil plan means the slice-aware
// path is active for this run.
func runAgent(ctx gocontext.Context, opts Opts, prog *program.Program, agent *program.AgentDef, plan *slices.Plan) AgentResult {
	repos := prog.EffectiveTargetRepos(agent)
	// Issue and PR refs are mutually exclusive at the CLI; use whichever is set
	// so each triggered build gets its own branch/PR.
	ref := opts.IssueRef
	if ref == "" {
		ref = opts.PRRef
	}
	taskID := buildTaskID(prog.ID, agent.Name, ref)
	multiRepo := len(repos) > 1

	// Start context tracking
	pt := opts.Tracker.StartPhase("agent:" + agent.Name)

	// Slice-aware dry run: N per-slice prompts replace the single
	// whole-issue prompt, which is skipped entirely so it does not
	// pollute context tracking.
	if opts.DryRun && plan != nil {
		printSlicePrompts(opts, prog, agent, plan, taskID, pt)
		pt.Finish(nil)
		return AgentResult{AgentName: agent.Name}
	}

	// Live slice-aware run: one fresh session per slice on a shared branch.
	if plan != nil {
		return runSliceLoop(ctx, opts, prog, agent, plan, taskID, repos, pt)
	}

	// Build prompt
	promptText := buildAgentPrompt(prog, agent, opts.PlanText, opts.IssueContext, opts.PRContext, opts.WorkspaceRoot, opts.Project, pt)

	if opts.DryRun {
		fmt.Printf("\n=== DRY RUN: Agent %s Prompt ===\n", agent.Name)
		fmt.Println(promptText)
		fmt.Println("=== END AGENT PROMPT ===")
		pt.Finish(nil)
		return AgentResult{AgentName: agent.Name}
	}

	// Save debug prompt
	if opts.DebugDir != "" {
		_ = os.MkdirAll(opts.DebugDir, 0755)
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
		defer func() { _ = os.RemoveAll(tmpDir) }()
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

	result, err := claudeRun(ctx, claude.Opts{
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
		if !runChecksWithRetry(ctx, opts, agent, claudeCWD, tools, agentVerification(agent, worktreePaths)) {
			slog.Error("checks still failing", "agent", agent.Name)
			cleanup()
			return AgentResult{AgentName: agent.Name, Error: fmt.Errorf("checks failed for agent %s", agent.Name)}
		}
	}

	prURLs, err := createAgentPRs(ctx, opts, prog, agent, taskID, claudeCWD, worktreeRepos)
	if err != nil {
		cleanup()
		return AgentResult{AgentName: agent.Name, Error: err}
	}

	cleanup()
	return AgentResult{AgentName: agent.Name, PRURLs: prURLs}
}

// runChecksWithRetry verifies the work and, when verification fails and the
// agent allows a retry, runs one fix-it session scoped to the failure output
// before it verifies a second time. The verification carries the labels for
// printed output, logs and debug artifacts.
func runChecksWithRetry(ctx gocontext.Context, opts Opts, agent *program.AgentDef, claudeCWD string, tools []string, v verification) bool {
	allPass, failedOutput := v.run()
	if allPass || !agent.RetryOnFail {
		return allPass
	}

	fmt.Printf("--- Checks failed for %s, retrying ---\n", v.scope)
	retryMaxTurns := agent.RetryMaxTurns
	if retryMaxTurns == 0 {
		retryMaxTurns = 15
	}

	retryPrompt := fmt.Sprintf("The following checks failed after your implementation. Please fix the issues:\n\n%s\n\nFix the errors and ensure all checks pass.", failedOutput)

	var retryLogFile string
	if opts.DebugDir != "" {
		retryLogFile = filepath.Join(opts.DebugDir, v.debugBase+"-retry-output.json")
	}

	retryResult, retryErr := claudeRun(ctx, claude.Opts{
		Prompt:       retryPrompt,
		CWD:          claudeCWD,
		MaxTurns:     retryMaxTurns,
		AllowedTools: strings.Join(tools, ","),
		LogFile:      retryLogFile,
	})
	if retryErr != nil {
		slog.Error("claude retry failed", append(v.logAttrs, "error", retryErr)...)
	} else if retryResult.IsError {
		slog.Warn("claude retry returned error", append(v.logAttrs, "subtype", retryResult.Subtype)...)
	}

	allPass, _ = v.run()
	return allPass
}

// createAgentPRs runs the shared PR tail: summarize the changes, create and
// cross-link PRs for every worktree repo, and record the URLs when
// MINION_PR_URLS_FILE is set.
func createAgentPRs(ctx gocontext.Context, opts Opts, prog *program.Program, agent *program.AgentDef, taskID, claudeCWD string, worktreeRepos []string) ([]string, error) {
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
		return nil, fmt.Errorf("project config required for PR creation")
	}

	// Summarize changes for PR title and description
	prTitle, prDescription := summarizeChanges(ctx, claudeCWD, prog, opts.IssueContext)

	prOpts := &pr.CreateOpts{
		AcceptanceCriteria: prog.AcceptanceCriteria,
		Source:             prog.Source,
	}
	prURLs, err := prCreateAndLinkAll(taskID, prTitle, prDescription, "", opts.WorkspaceRoot, labelsCSV, worktreeRepos, fullNameFn, principalRepo, prOpts)
	if err != nil {
		return nil, fmt.Errorf("PR creation failed: %w", err)
	}

	// Write PR URLs to file if env var set
	if prURLsFile := os.Getenv("MINION_PR_URLS_FILE"); prURLsFile != "" {
		if f, err := os.OpenFile(prURLsFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644); err == nil {
			for _, u := range prURLs {
				_, _ = fmt.Fprintln(f, u)
			}
			_ = f.Close()
		}
	}

	return prURLs, nil
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
				_ = os.RemoveAll(virtualWS)
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
		// Everything may already be committed (the slice loop commits and
		// pushes each slice); diff the branch against origin's default.
		diffCmd = exec.Command("git", "diff", "origin/HEAD...HEAD")
		diffCmd.Dir = cwd
		diffOut, _ = diffCmd.Output()
	}
	if len(diffOut) == 0 {
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
	result, err := claudeRun(ctx, claude.Opts{
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
		output, err := checksRun(wtPath)
		if err != nil {
			allPass = false
			// The failure reason frequently lives only in err: an exec-level
			// failure (e.g. `make` not resolvable in the runner's PATH) makes
			// CombinedOutput return empty output with the real cause in err.
			// Log it and fold it into the report so the run log shows why and
			// the retry agent has something concrete to fix instead of an
			// empty prompt.
			slog.Error("check failed", "repo", filepath.Base(wtPath), "error", err)
			if s := strings.TrimSpace(output); s != "" {
				failed.WriteString(s)
				failed.WriteByte('\n')
			}
			fmt.Fprintf(&failed, "%v\n", err)
		}
	}
	return allPass, failed.String()
}
