package executor

import (
	gocontext "context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/partio-io/minions/internal/claude"
	pcontext "github.com/partio-io/minions/internal/context"
	"github.com/partio-io/minions/internal/git"
	"github.com/partio-io/minions/internal/program"
	"github.com/partio-io/minions/internal/slices"
	"github.com/partio-io/minions/internal/worktree"
)

// resolveSlicePlan returns the parsed slice plan when the slice-aware path
// applies: the program opted in via `slices: true` AND the issue carries a
// plan comment. A present-but-malformed plan is an error — never a silent
// fallback to the whole-issue prompt.
func resolveSlicePlan(prog *program.Program, comments []slices.Comment) (*slices.Plan, error) {
	if !prog.Slices {
		return nil, nil
	}
	c, ok := slices.FindPlanComment(comments)
	if !ok {
		return nil, nil
	}
	plan, err := slices.Parse(c.Body)
	if err != nil {
		return nil, fmt.Errorf("parsing slice-plan comment: %w", err)
	}
	return plan, nil
}

// printSlicePrompts renders the dry-run output for a slice-aware program:
// one full agent prompt per slice instead of a single whole-issue prompt.
func printSlicePrompts(opts Opts, prog *program.Program, agent *program.AgentDef, plan *slices.Plan, pt *pcontext.PhaseTracker) {
	prdComment, _ := slices.FindPRDComment(opts.IssueComments)
	for i := range plan.Slices {
		sliceCtx := buildSliceIssueContext(opts.IssueTitle, opts.IssueBody, prdComment, plan, i)
		promptText := buildAgentPrompt(prog, agent, opts.PlanText, sliceCtx, opts.PRContext, opts.WorkspaceRoot, opts.Project, pt)
		fmt.Printf("\n=== DRY RUN: Agent %s Prompt — Slice %d/%d ===\n", agent.Name, i+1, len(plan.Slices))
		fmt.Println(promptText)
		fmt.Println("=== END AGENT PROMPT ===")
	}
}

// runSliceLoop executes a slice-aware live run: one fresh Claude session per
// slice, each in a worktree created at the shared branch's current tip, with
// checks after every slice, an empty marker commit + push recording
// completion, and a single PR created only once the final slice passes. A
// failing slice aborts the run; completed slices stay pushed and no PR is
// opened.
func runSliceLoop(ctx gocontext.Context, opts Opts, prog *program.Program, agent *program.AgentDef, plan *slices.Plan, taskID string, repos []string, pt *pcontext.PhaseTracker) AgentResult {
	prdComment, _ := slices.FindPRDComment(opts.IssueComments)
	total := len(plan.Slices)
	branchName := "minion/" + taskID
	multiRepo := len(repos) > 1

	tools := agent.Tools
	if len(tools) == 0 {
		tools = []string{"Edit", "Write", "Read", "Glob", "Grep", "Bash"}
	}
	maxTurns := agent.MaxTurns
	if maxTurns == 0 {
		maxTurns = 30
	}
	mcpServers := buildMCPServers(agent.MCPs)

	cleanup := func() {
		for _, repo := range repos {
			worktree.Cleanup(filepath.Join(opts.WorkspaceRoot, repo), taskID)
		}
	}
	fail := func(err error) AgentResult {
		cleanup()
		pt.Finish(nil)
		return AgentResult{AgentName: agent.Name, Error: err}
	}

	// Resume: a re-triggered build continues where the branch on origin left
	// off instead of starting from zero. The decision comes only from fetched
	// remote state — never from whatever a stale local clone contains.
	completed, onOrigin, err := resumeFromOrigin(opts.WorkspaceRoot, repos, branchName, total)
	if err != nil {
		return fail(err)
	}
	if onOrigin {
		slog.Info("branch found on origin, resuming", "branch", branchName, "completed", completed, "total", total)
	}

	// Every slice already marked complete: the only work possibly missing is
	// the PR itself (a previous run may have died between the final push and
	// PR creation). Ensure it exists and exit — re-running a finished build
	// is a no-op, not a duplicate.
	if onOrigin && completed == total {
		prURLs, err := ensurePRs(ctx, opts, prog, agent, taskID, repos, branchName, multiRepo)
		if err != nil {
			return fail(err)
		}
		cleanup()
		pt.Finish(nil)
		return AgentResult{AgentName: agent.Name, PRURLs: prURLs}
	}

	var worktreePaths, worktreeRepos []string
	var claudeCWD string

	for i := completed; i < len(plan.Slices); i++ {
		s := &plan.Slices[i]
		num := i + 1
		fmt.Printf("--- Slice %d/%d: %s ---\n", num, total, s.Title)

		// Fresh worktrees at the branch's current tip: a resumed run's first
		// slice checks out from the fetched origin branch, a fresh run's
		// slice one creates the branch, and later slices re-attach to the
		// local branch carrying the earlier slices' commits. Every creation
		// path removes the previous slice's worktree.
		worktreePaths, worktreeRepos = nil, nil
		for _, repo := range repos {
			repoPath := filepath.Join(opts.WorkspaceRoot, repo)
			if _, err := os.Stat(repoPath); os.IsNotExist(err) {
				if multiRepo {
					slog.Warn("repo not found, skipping", "path", repoPath)
					continue
				}
				return fail(fmt.Errorf("repo not found at %s", repoPath))
			}
			create := worktree.Create
			switch {
			case onOrigin && i == completed:
				create = worktree.CreateFromOrigin
			case num > 1:
				create = worktree.CreateAtBranch
			}
			wtPath, err := create(repoPath, taskID)
			if err != nil {
				return fail(fmt.Errorf("slice %d/%d: creating worktree for %s: %w", num, total, repo, err))
			}
			worktreePaths = append(worktreePaths, wtPath)
			worktreeRepos = append(worktreeRepos, repo)
		}
		if len(worktreePaths) == 0 {
			return fail(fmt.Errorf("slice %d/%d: no worktrees created", num, total))
		}

		var tmpDir string
		var err error
		claudeCWD, tmpDir, err = buildCWD(worktreePaths, worktreeRepos)
		if err != nil {
			return fail(err)
		}
		if tmpDir != "" {
			defer os.RemoveAll(tmpDir)
		}

		sliceCtx := buildSliceIssueContext(opts.IssueTitle, opts.IssueBody, prdComment, plan, i)
		promptText := buildAgentPrompt(prog, agent, opts.PlanText, sliceCtx, opts.PRContext, opts.WorkspaceRoot, opts.Project, pt)

		var logFile string
		if opts.DebugDir != "" {
			os.MkdirAll(opts.DebugDir, 0755)
			_ = os.WriteFile(filepath.Join(opts.DebugDir, fmt.Sprintf("agent-%s-slice-%d-prompt.md", agent.Name, num)), []byte(promptText), 0644)
			logFile = filepath.Join(opts.DebugDir, fmt.Sprintf("agent-%s-slice-%d-output.json", agent.Name, num))
		}

		fmt.Printf("--- Running Claude for agent %s (slice %d/%d) ---\n", agent.Name, num, total)
		spt := opts.Tracker.StartPhase(fmt.Sprintf("agent:%s:slice-%d", agent.Name, num))
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
			spt.Finish(nil)
			return fail(fmt.Errorf("slice %d/%d: claude failed: %w", num, total, err))
		}
		spt.Finish(&pcontext.InvocationMetrics{
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
			slog.Warn("claude returned error result", "agent", agent.Name, "slice", num, "subtype", result.Subtype)
		}

		if agent.Checks {
			if !runChecksWithRetry(ctx, opts, agent, worktreePaths, claudeCWD, tools, num, total) {
				postSliceFailureComment(opts, num, total, s.Title, branchName)
				return fail(fmt.Errorf("slice %d/%d: checks failed after retry", num, total))
			}
		}

		// Record completion: commit whatever the session left uncommitted,
		// add the empty marker commit, and push so the next slice's worktree
		// starts from this tip.
		for _, wtPath := range worktreePaths {
			if err := commitSliceWork(wtPath, num, total, s.Title); err != nil {
				return fail(fmt.Errorf("slice %d/%d: %w", num, total, err))
			}
			if err := git.CommitEmpty(wtPath, slices.MarkerSubject(num, total)); err != nil {
				return fail(fmt.Errorf("slice %d/%d: marker commit: %w", num, total, err))
			}
			if err := git.Push(wtPath, branchName); err != nil {
				return fail(fmt.Errorf("slice %d/%d: pushing %s: %w", num, total, branchName, err))
			}
		}
	}

	// PR creation — once, after the final slice passed its checks. The final
	// slice's worktrees are still in place for the diff and the push.
	prURLs, err := createAgentPRs(ctx, opts, prog, agent, taskID, claudeCWD, worktreeRepos)
	if err != nil {
		return fail(err)
	}

	cleanup()
	pt.Finish(nil)
	return AgentResult{AgentName: agent.Name, PRURLs: prURLs}
}

// ensurePRs finishes a build whose slices are all complete on origin. Repos
// whose PR is already open are left alone and report their existing URL;
// repos without one get a worktree checked out from the fetched branch and go
// through the normal PR tail. No build session runs either way.
func ensurePRs(ctx gocontext.Context, opts Opts, prog *program.Program, agent *program.AgentDef, taskID string, repos []string, branchName string, multiRepo bool) ([]string, error) {
	var urls, missingRepos []string
	for _, repo := range repos {
		repoPath := filepath.Join(opts.WorkspaceRoot, repo)
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			if multiRepo {
				slog.Warn("repo not found, skipping", "path", repoPath)
				continue
			}
			return nil, fmt.Errorf("repo not found at %s", repoPath)
		}
		url, err := prURLForBranch(repoPath, branchName)
		if err != nil {
			return nil, fmt.Errorf("checking for an existing PR in %s: %w", repo, err)
		}
		if url != "" {
			slog.Info("build already complete, PR exists", "repo", repo, "url", url)
			urls = append(urls, url)
			continue
		}
		missingRepos = append(missingRepos, repo)
	}
	if len(missingRepos) == 0 {
		return urls, nil
	}

	var wtPaths, wtRepos []string
	for _, repo := range missingRepos {
		wtPath, err := worktree.CreateFromOrigin(filepath.Join(opts.WorkspaceRoot, repo), taskID)
		if err != nil {
			return nil, fmt.Errorf("creating worktree for %s: %w", repo, err)
		}
		wtPaths = append(wtPaths, wtPath)
		wtRepos = append(wtRepos, repo)
	}
	claudeCWD, tmpDir, err := buildCWD(wtPaths, wtRepos)
	if err != nil {
		return nil, err
	}
	if tmpDir != "" {
		defer os.RemoveAll(tmpDir)
	}
	created, err := createAgentPRs(ctx, opts, prog, agent, taskID, claudeCWD, wtRepos)
	if err != nil {
		return nil, err
	}
	return append(urls, created...), nil
}

// resumeFromOrigin fetches branchName's state on origin for every available
// repo and reports how many slices a prior run already completed there, plus
// whether the branch exists on origin at all. Local-only branch state is
// never consulted. Repos that disagree with each other are a loud error —
// resuming from a guess could silently rebuild or skip a slice.
func resumeFromOrigin(workspaceRoot string, repos []string, branchName string, total int) (int, bool, error) {
	completed, onOrigin, seen := 0, false, false
	for _, repo := range repos {
		repoPath := filepath.Join(workspaceRoot, repo)
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			continue // the slice loop skips or fails missing repos itself
		}
		exists, err := git.RemoteBranchExists(repoPath, branchName)
		if err != nil {
			return 0, false, fmt.Errorf("checking origin for %s in %s: %w", branchName, repo, err)
		}
		done := 0
		if exists {
			if err := git.FetchBranch(repoPath, branchName); err != nil {
				return 0, false, fmt.Errorf("fetching %s in %s: %w", branchName, repo, err)
			}
			subjects, err := git.ListCommitSubjects(repoPath, "origin/"+branchName)
			if err != nil {
				return 0, false, fmt.Errorf("listing commits of origin/%s in %s: %w", branchName, repo, err)
			}
			if done, err = slices.ResumePoint(subjects, total); err != nil {
				return 0, false, fmt.Errorf("resume point of %s in %s: %w", branchName, repo, err)
			}
		}
		if seen && (exists != onOrigin || done != completed) {
			return 0, false, fmt.Errorf("repos disagree on %s's completed slices; not resuming from a guess", branchName)
		}
		completed, onOrigin, seen = done, exists, true
	}
	return completed, onOrigin, nil
}

// commitSliceWork stages and commits everything the slice's session left
// uncommitted — losing uncommitted work when the next slice replaces the
// worktree is the failure this prevents. Sessions that committed themselves
// leave a clean tree and need nothing here. The subject deliberately does not
// start with the marker prefix so marker counting never over-counts.
func commitSliceWork(wtPath string, num, total int, title string) error {
	status, err := git.ExecGitDir(wtPath, "status", "--porcelain")
	if err != nil {
		return fmt.Errorf("checking worktree status: %w", err)
	}
	if strings.TrimSpace(status) == "" {
		return nil
	}
	if _, err := git.ExecGitDir(wtPath, "add", "-A"); err != nil {
		return fmt.Errorf("staging slice work: %w", err)
	}
	if _, err := git.ExecGitDir(wtPath, "commit", "-m", fmt.Sprintf("slice %d/%d: %s", num, total, title)); err != nil {
		return fmt.Errorf("committing slice work: %w", err)
	}
	return nil
}

// buildSliceIssueContext renders the bounded per-slice issue context that
// replaces the whole-discussion blob: issue title and body, the PRD comment
// when present, the full plan for orientation, and the directive naming the
// one slice to build.
func buildSliceIssueContext(issueTitle, issueBody, prdComment string, plan *slices.Plan, idx int) string {
	s := plan.Slices[idx]
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s\n\n", issueTitle, issueBody)
	if prdComment != "" {
		b.WriteString("## PRD (minion:research comment)\n\n" + strings.TrimSpace(prdComment) + "\n\n")
	}
	b.WriteString("## Slice Plan\n\nThis issue is built one slice at a time. The full plan, for orientation only:\n\n")
	b.WriteString(strings.TrimSpace(plan.Raw) + "\n\n")
	fmt.Fprintf(&b, "## Your Slice\n\nBuild only this slice: Slice %d — %s\n\n", s.Number, s.Title)
	b.WriteString("Implement nothing from any other slice; other slices are built in their own sessions.\n")
	return b.String()
}
