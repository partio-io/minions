package pr

import (
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/partio-io/minions/internal/git"
)

// CreateOpts holds optional fields for PR creation.
type CreateOpts struct {
	Source             string   // e.g., "partio-io/cli#3" — referenced in PR body
	AcceptanceCriteria []string // listed in PR body
}

// aheadOfOriginDefault reports whether HEAD carries commits beyond origin's
// default branch. A branch that was already fully pushed (the slice loop
// pushes after every slice) has no unpushed commits, yet still needs its PR —
// "no changes" must mean nothing beyond the default branch, not nothing
// unpushed. When origin/HEAD is not resolvable this reports false, keeping
// the historical unpushed-only behavior.
func aheadOfOriginDefault(worktreePath string) bool {
	out, err := git.ExecGitDir(worktreePath, "log", "HEAD", "--not", "origin/HEAD", "--oneline")
	return err == nil && strings.TrimSpace(out) != ""
}

// openPRURLForBranch returns the URL of the open PR whose head is branch in
// repoFullName, or "" when none exists. A package-level seam so tests can
// stub the gh call.
var openPRURLForBranch = func(repoFullName, branch string) (string, error) {
	out, err := exec.Command("gh", "pr", "list", "--repo", repoFullName, "--head", branch, "--state", "open", "--json", "url", "--jq", ".[0].url // empty").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("gh pr list --head %s: %s: %w", branch, strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// Create stages, commits, pushes, and creates a PR for a minion's work.
// Handles both uncommitted changes (stages + commits) and pre-committed changes (just pushes).
// principalRepo is the full name of the principal repo (used in commit messages/PR bodies).
// Returns the PR URL or empty string if no changes.
func Create(worktreePath, repoFullName, taskID, title, description, why string, labels []string, principalRepo string, opts *CreateOpts) (string, error) {
	branchName := "minion/" + taskID

	// Check for uncommitted changes
	status, _ := git.ExecGitDir(worktreePath, "status", "--porcelain")
	hasUncommitted := strings.TrimSpace(status) != ""

	// Check for new commits (Claude may have committed already)
	logOut, _ := git.ExecGitDir(worktreePath, "log", "HEAD", "--not", "--remotes", "--oneline")
	hasNewCommits := strings.TrimSpace(logOut) != ""

	if !hasUncommitted && !hasNewCommits && !aheadOfOriginDefault(worktreePath) {
		slog.Info("no changes", "repo", filepath.Base(worktreePath))
		return "", nil
	}

	// Stage and commit only if there are uncommitted changes
	if hasUncommitted {
		if _, err := git.ExecGitDir(worktreePath, "add", "-A"); err != nil {
			return "", fmt.Errorf("staging changes: %w", err)
		}

		commitMsg := fmt.Sprintf("%s\n\nAutomated by %s (task: %s)\n\nCo-Authored-By: Claude <noreply@anthropic.com>", title, principalRepo, taskID)
		if _, err := git.ExecGitDir(worktreePath, "commit", "-m", commitMsg); err != nil {
			return "", fmt.Errorf("committing changes: %w", err)
		}
	}

	// Push
	if _, err := git.ExecGitDir(worktreePath, "push", "--force-with-lease", "-u", "origin", branchName); err != nil {
		return "", fmt.Errorf("pushing branch: %w", err)
	}

	// A session may have opened the PR itself despite instructions, or a
	// previous run may have died between creation and completion. Adopt an
	// existing open PR instead of failing gh pr create with "already
	// exists" — the push above already brought it up to date.
	if url, err := openPRURLForBranch(repoFullName, branchName); err != nil {
		slog.Warn("checking for existing PR failed; attempting create", "branch", branchName, "error", err)
	} else if url != "" {
		slog.Info("adopting existing open PR", "branch", branchName, "url", url)
		return url, nil
	}

	// Build PR body
	var bodyBuilder strings.Builder

	if description != "" {
		bodyBuilder.WriteString("## Objective\n\n")
		bodyBuilder.WriteString(description)
		bodyBuilder.WriteString("\n\n")
	}

	if why != "" {
		bodyBuilder.WriteString("## Why\n\n")
		bodyBuilder.WriteString(why)
		bodyBuilder.WriteString("\n\n")
	}

	if opts != nil && len(opts.AcceptanceCriteria) > 0 {
		bodyBuilder.WriteString("## Acceptance Criteria\n\n")
		for _, c := range opts.AcceptanceCriteria {
			fmt.Fprintf(&bodyBuilder, "- [ ] %s\n", c)
		}
		bodyBuilder.WriteString("\n")
	}

	if opts != nil && opts.Source != "" {
		bodyBuilder.WriteString("## Source\n\n")
		if strings.Contains(opts.Source, "#") {
			fmt.Fprintf(&bodyBuilder, "Resolves %s\n\n", opts.Source)
		} else {
			fmt.Fprintf(&bodyBuilder, "%s\n\n", opts.Source)
		}
	}

	bodyBuilder.WriteString("---\n\n")
	fmt.Fprintf(&bodyBuilder, "Automated PR by [%s](https://github.com/%s) · Task: `%s`\n\n", principalRepo, principalRepo, taskID)
	bodyBuilder.WriteString("*Created by an unattended coding agent. Please review carefully.*")

	prBody := bodyBuilder.String()

	// Ensure labels exist in target repo (ignore errors for already-existing labels)
	for _, l := range labels {
		create := exec.Command("gh", "label", "create", l, "--repo", repoFullName)
		_ = create.Run()
	}

	args := []string{
		"pr", "create",
		"--repo", repoFullName,
		"--head", branchName,
		"--title", "[minion] " + title,
		"--body", prBody,
	}
	for _, l := range labels {
		args = append(args, "--label", l)
	}

	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("creating PR: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return strings.TrimSpace(string(out)), nil
}
