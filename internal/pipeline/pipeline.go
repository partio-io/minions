package pipeline

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/partio-io/minions/internal/checks"
	"github.com/partio-io/minions/internal/claude"
	"github.com/partio-io/minions/internal/git"
	"github.com/partio-io/minions/internal/pr"
	"github.com/partio-io/minions/internal/worktree"
)

// Def configures a single pipeline execution.
type Def struct {
	// Identity
	Name   string
	TaskID string

	// Worktree setup
	WorkspaceRoot string
	TargetRepos   []string

	// Claude invocation
	PromptText   string
	PlanText     string // if set, prepended to prompt as implementation plan context
	MaxTurns     int
	AllowedTools string // comma-separated, e.g. "Edit,Write,Read,Glob,Grep,Bash"

	// Post-Claude checks
	SkipMarker    string // file in worktree that signals "no work needed"
	RunChecks     bool
	RetryOnFail   bool
	RetryMaxTurns int

	// PR creation
	CreatePR   bool
	CommitMsg  string   // for single-repo: full commit message
	PRTitle    string   // for single-repo: PR title
	PRBody     string   // for single-repo: PR body
	PRLabels   []string // labels to apply
	PRRepo     string   // for single-repo: full repo name (e.g. "partio-io/docs")
	StageFiles []string // for single-repo: specific files to stage; empty = git add -A

	// Multi-repo task fields (used by pr.CreateAndLinkAll)
	TaskTitle          string
	TaskDescription    string
	TaskWhy            string
	TaskSource         string   // e.g., "partio-io/cli#3"
	AcceptanceCriteria []string // included in PR body

	// Cross-linking
	SourcePRRepo   string
	SourcePRNumber string

	// Multi-repo resolution
	FullNameFn    pr.FullNameFunc // resolves short repo name to full GitHub name
	PrincipalRepo string         // full name of the principal repo (e.g. "partio-io/cli")

	// Control
	DryRun   bool
	DebugDir string
}

// Result holds the outcome of a pipeline execution.
type Result struct {
	PRURLs     []string
	Skipped    bool
	SkipReason string
}

// Execute runs the full pipeline: worktree -> claude -> checks -> PR.
func Execute(ctx context.Context, def Def) (*Result, error) {
	if def.AllowedTools == "" {
		def.AllowedTools = "Edit,Write,Read,Glob,Grep,Bash"
	}
	if def.MaxTurns == 0 {
		def.MaxTurns = 30
	}

	multiRepo := len(def.TargetRepos) > 1

	// Save debug prompt
	if def.DebugDir != "" {
		os.MkdirAll(def.DebugDir, 0755)
		_ = os.WriteFile(filepath.Join(def.DebugDir, "prompt.md"), []byte(def.PromptText), 0644)
	}

	if def.DryRun {
		fmt.Println()
		fmt.Println("=== DRY RUN: Generated Prompt ===")
		fmt.Println(def.PromptText)
		fmt.Println("=== END PROMPT ===")
		fmt.Println()
		fmt.Printf("Would create worktrees for: %v\n", def.TargetRepos)
		fmt.Printf("Would run: claude --max-turns %d --allowedTools %s\n", def.MaxTurns, def.AllowedTools)
		return &Result{}, nil
	}

	// Create worktrees
	fmt.Println("--- Creating worktrees ---")
	var worktreePaths []string
	var worktreeRepos []string
	for _, repo := range def.TargetRepos {
		repoPath := filepath.Join(def.WorkspaceRoot, repo)
		if _, err := os.Stat(repoPath); os.IsNotExist(err) {
			if multiRepo {
				slog.Warn("repo not found, skipping", "path", repoPath)
				continue
			}
			return nil, fmt.Errorf("repo not found at %s", repoPath)
		}
		wtPath, err := worktree.Create(repoPath, def.TaskID)
		if err != nil {
			return nil, fmt.Errorf("creating worktree for %s: %w", repo, err)
		}
		worktreePaths = append(worktreePaths, wtPath)
		worktreeRepos = append(worktreeRepos, repo)
		fmt.Printf("Worktree: %s\n", wtPath)
	}

	if len(worktreePaths) == 0 {
		return nil, fmt.Errorf("no worktrees created")
	}

	cleanup := func() {
		for _, repo := range def.TargetRepos {
			worktree.Cleanup(filepath.Join(def.WorkspaceRoot, repo), def.TaskID)
		}
	}

	// Determine CWD for Claude
	var claudeCWD string
	if multiRepo {
		virtualWS, err := os.MkdirTemp("", "minion-workspace-*")
		if err != nil {
			cleanup()
			return nil, fmt.Errorf("creating virtual workspace: %w", err)
		}
		defer os.RemoveAll(virtualWS)

		for i, repo := range worktreeRepos {
			if err := os.Symlink(worktreePaths[i], filepath.Join(virtualWS, repo)); err != nil {
				cleanup()
				return nil, fmt.Errorf("creating symlink for %s: %w", repo, err)
			}
		}
		claudeCWD = virtualWS
	} else {
		claudeCWD = worktreePaths[0]
	}

	// Prepend plan context if available
	if def.PlanText != "" {
		def.PromptText = "## Implementation Plan\n\nFollow this plan that was reviewed and approved:\n\n" + def.PlanText + "\n\n---\n\n" + def.PromptText
	}

	// Run Claude
	fmt.Println("--- Running Claude Code ---")
	var logFile string
	if def.DebugDir != "" {
		logFile = filepath.Join(def.DebugDir, "claude-output.json")
	}
	result, err := claude.Run(ctx, claude.Opts{
		Prompt:       def.PromptText,
		CWD:          claudeCWD,
		MaxTurns:     def.MaxTurns,
		AllowedTools: def.AllowedTools,
		LogFile:      logFile,
	})
	if err != nil {
		// Process crashed entirely — no result, no changes possible
		cleanup()
		return nil, fmt.Errorf("claude failed: %w", err)
	}
	if result.IsError {
		// Claude returned an error (e.g., max turns exceeded) but may have produced changes
		slog.Warn("claude returned error result", "subtype", result.Subtype)
	}

	// Check skip marker
	if def.SkipMarker != "" {
		for _, wtPath := range worktreePaths {
			markerPath := filepath.Join(wtPath, def.SkipMarker)
			if data, err := os.ReadFile(markerPath); err == nil {
				fmt.Printf("Claude determined no update is needed:\n%s\n", string(data))
				cleanup()
				return &Result{Skipped: true, SkipReason: strings.TrimSpace(string(data))}, nil
			}
		}
	}

	// Check for changes
	hasChanges := false
	for i, wtPath := range worktreePaths {
		status, _ := git.ExecGitDir(wtPath, "status", "--porcelain")
		if strings.TrimSpace(status) != "" {
			hasChanges = true
			slog.Info("worktree has changes", "repo", worktreeRepos[i])
		} else {
			slog.Info("worktree has no changes", "repo", worktreeRepos[i])
		}
	}
	if !hasChanges {
		fmt.Println("No changes produced.")
		cleanup()
		return &Result{Skipped: true, SkipReason: "no changes"}, nil
	}

	// Run checks
	if def.RunChecks {
		allPass, failedOutput := runChecks(worktreePaths)

		if !allPass && def.RetryOnFail {
			fmt.Println("--- Checks failed, retrying with error feedback ---")
			retryMaxTurns := def.RetryMaxTurns
			if retryMaxTurns == 0 {
				retryMaxTurns = 15
			}

			var retryLogFile string
			if def.DebugDir != "" {
				retryLogFile = filepath.Join(def.DebugDir, "claude-retry-output.json")
			}

			retryPrompt := fmt.Sprintf("The following checks failed after your implementation. Please fix the issues:\n\n%s\n\nFix the errors and ensure all checks pass. Do not introduce new changes beyond what's needed to fix the failures.", failedOutput)
			retryResult, retryErr := claude.Run(ctx, claude.Opts{
				Prompt:       retryPrompt,
				CWD:          claudeCWD,
				MaxTurns:     retryMaxTurns,
				AllowedTools: def.AllowedTools,
				LogFile:      retryLogFile,
			})
			if retryErr != nil {
				slog.Error("claude retry failed", "error", retryErr)
			} else if retryResult.IsError {
				slog.Warn("claude retry returned error result", "subtype", retryResult.Subtype)
			}

			fmt.Println("--- Re-running checks after retry ---")
			allPass, _ = runChecks(worktreePaths)
		}

		if !allPass {
			slog.Error("checks still failing, skipping PR creation")
			cleanup()
			return nil, fmt.Errorf("checks failed for %s", def.TaskID)
		}
	}

	// Create PRs
	var prURLs []string
	if def.CreatePR {
		fmt.Println("--- Creating PRs ---")

		if multiRepo || def.CommitMsg == "" {
			labelsCSV := strings.Join(def.PRLabels, ",")
			if labelsCSV == "" {
				labelsCSV = "minion"
			}
			if def.FullNameFn == nil || def.PrincipalRepo == "" {
				cleanup()
				return nil, fmt.Errorf("project config required: FullNameFn and PrincipalRepo must be set (ensure .minions/project.yaml exists)")
			}
			prOpts := &pr.CreateOpts{
				Source:             def.TaskSource,
				AcceptanceCriteria: def.AcceptanceCriteria,
			}
			urls, err := pr.CreateAndLinkAll(def.TaskID, def.TaskTitle, def.TaskDescription, def.TaskWhy, def.WorkspaceRoot, labelsCSV, worktreeRepos, def.FullNameFn, def.PrincipalRepo, prOpts)
			if err != nil {
				cleanup()
				return nil, fmt.Errorf("PR creation failed: %w", err)
			}
			prURLs = urls
		} else {
			url, err := createSingleRepoPR(def, worktreePaths[0])
			if err != nil {
				cleanup()
				return nil, err
			}
			if url != "" {
				prURLs = append(prURLs, url)
			}
		}

		if len(prURLs) == 0 {
			cleanup()
			return nil, fmt.Errorf("no PRs created for %s", def.TaskID)
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
	}

	// Cleanup
	fmt.Println("--- Cleaning up ---")
	cleanup()

	return &Result{PRURLs: prURLs}, nil
}

func runChecks(worktreePaths []string) (bool, string) {
	fmt.Println("--- Running checks ---")
	allPass := true
	var failedOutput string
	for _, wtPath := range worktreePaths {
		output, err := checks.Run(wtPath)
		if err != nil {
			allPass = false
			failedOutput += output + "\n"
		}
	}
	return allPass, failedOutput
}

func createSingleRepoPR(def Def, wtPath string) (string, error) {
	branchName := "minion/" + def.TaskID

	// Stage
	if len(def.StageFiles) > 0 {
		for _, f := range def.StageFiles {
			if _, err := git.ExecGitDir(wtPath, "add", f); err != nil {
				return "", fmt.Errorf("staging %s: %w", f, err)
			}
		}
	} else {
		if _, err := git.ExecGitDir(wtPath, "add", "-A"); err != nil {
			return "", fmt.Errorf("staging changes: %w", err)
		}
	}

	// Commit
	if _, err := git.ExecGitDir(wtPath, "commit", "-m", def.CommitMsg); err != nil {
		return "", fmt.Errorf("committing: %w", err)
	}

	// Push
	if _, err := git.ExecGitDir(wtPath, "push", "-u", "origin", branchName); err != nil {
		return "", fmt.Errorf("pushing: %w", err)
	}

	// Ensure labels exist
	for _, l := range def.PRLabels {
		create := exec.Command("gh", "label", "create", l, "--repo", def.PRRepo)
		_ = create.Run()
	}

	// Create PR
	args := []string{
		"pr", "create",
		"--repo", def.PRRepo,
		"--head", branchName,
		"--title", def.PRTitle,
		"--body", def.PRBody,
	}
	for _, l := range def.PRLabels {
		args = append(args, "--label", l)
	}

	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("creating PR: %s: %w", strings.TrimSpace(string(out)), err)
	}
	prURL := strings.TrimSpace(string(out))

	// Cross-link to source PR
	if def.SourcePRRepo != "" && def.SourcePRNumber != "" {
		fmt.Println("--- Cross-linking PRs ---")
		pr.CommentOnPR(def.SourcePRRepo, def.SourcePRNumber, fmt.Sprintf("Related PR: %s", prURL))
	}

	return prURL, nil
}
