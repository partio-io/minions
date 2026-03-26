package plan

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	claudesdk "github.com/partio-io/claude-agent-sdk-go"

	"github.com/partio-io/minions/internal/project"
	"github.com/partio-io/minions/internal/prompt"
	"github.com/partio-io/minions/internal/task"
	"github.com/partio-io/minions/internal/worktree"
)

// Opts configures a plan generation run.
type Opts struct {
	Task          *task.Task
	WorkspaceRoot string
	MaxTurns      int              // default 15
	Feedback      string           // human feedback from issue comments (for replan)
	PreviousPlan  string           // the prior plan text (for replan)
	Project       *project.Project // may be nil for backward compat
}

// Result holds the output of a plan generation.
type Result struct {
	Plan      string // the implementation plan
	Questions string // questions for the human (if any)
	NumTurns  int
	CostUSD   float64
}

// Generate runs Claude in plan mode against the task's target repos.
// Claude explores the codebase read-only and produces an implementation plan.
func Generate(ctx context.Context, opts Opts) (*Result, error) {
	if opts.MaxTurns == 0 {
		opts.MaxTurns = 15
	}

	t := opts.Task

	// Build plan prompt
	planPrompt, err := prompt.BuildPlan(t, opts.WorkspaceRoot, opts.Feedback, opts.PreviousPlan, opts.Project)
	if err != nil {
		return nil, fmt.Errorf("building plan prompt: %w", err)
	}

	// Create worktrees for target repos
	var worktreePaths []string
	var worktreeRepos []string
	for _, repo := range t.TargetRepos {
		repoPath := filepath.Join(opts.WorkspaceRoot, repo)
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			slog.Warn("repo not found, skipping", "path", repoPath)
			continue
		}
		taskID := "plan-" + t.ID
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
		for _, repo := range t.TargetRepos {
			worktree.Cleanup(filepath.Join(opts.WorkspaceRoot, repo), "plan-"+t.ID)
		}
	}
	defer cleanup()

	// Determine CWD
	var claudeCWD string
	if len(worktreePaths) > 1 {
		virtualWS, err := os.MkdirTemp("", "minion-plan-workspace-*")
		if err != nil {
			return nil, fmt.Errorf("creating virtual workspace: %w", err)
		}
		defer os.RemoveAll(virtualWS)

		for i, repo := range worktreeRepos {
			if err := os.Symlink(worktreePaths[i], filepath.Join(virtualWS, repo)); err != nil {
				return nil, fmt.Errorf("creating symlink for %s: %w", repo, err)
			}
		}
		claudeCWD = virtualWS
	} else {
		claudeCWD = worktreePaths[0]
	}

	// Run Claude in plan mode (read-only)
	slog.Info("running claude in plan mode", "task", t.ID, "cwd", claudeCWD, "max_turns", opts.MaxTurns)

	sdkOpts := []claudesdk.Option{
		claudesdk.WithCwd(claudeCWD),
		claudesdk.WithMaxTurns(opts.MaxTurns),
		claudesdk.WithPermissionMode("plan"),
		claudesdk.WithAllowedTools("Read", "Glob", "Grep", "Bash"),
	}

	resultMsg, err := claudesdk.Prompt(ctx, planPrompt, sdkOpts...)
	if err != nil {
		return nil, fmt.Errorf("claude plan failed: %w", err)
	}

	var costUSD float64
	if resultMsg.TotalCostUSD != nil {
		costUSD = *resultMsg.TotalCostUSD
	}

	slog.Info("plan completed",
		"task", t.ID,
		"turns", resultMsg.NumTurns,
		"cost_usd", costUSD,
		"subtype", resultMsg.Subtype,
	)

	var resultText string
	if resultMsg.Result != nil {
		resultText = *resultMsg.Result
	}

	// Parse plan and questions from result text
	planText, questions := parsePlanResult(resultText)

	return &Result{
		Plan:      planText,
		Questions: questions,
		NumTurns:  resultMsg.NumTurns,
		CostUSD:   costUSD,
	}, nil
}

// parsePlanResult splits the result text into plan and questions sections.
func parsePlanResult(text string) (plan, questions string) {
	// Look for a Questions section
	questionsMarkers := []string{
		"### Questions",
		"## Questions",
	}

	for _, marker := range questionsMarkers {
		idx := strings.Index(text, marker)
		if idx != -1 {
			plan = strings.TrimSpace(text[:idx])
			questions = strings.TrimSpace(text[idx+len(marker):])
			// Remove "No questions." or similar
			if strings.Contains(strings.ToLower(questions), "no questions") {
				questions = ""
			}
			return plan, questions
		}
	}

	return strings.TrimSpace(text), ""
}
