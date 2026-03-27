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

// Create stages, commits, pushes, and creates a PR for a minion's work.
// principalRepo is the full name of the principal repo (used in commit messages/PR bodies).
// Returns the PR URL or empty string if no changes.
func Create(worktreePath, repoFullName, taskID, title, description, why string, labels []string, principalRepo string, opts *CreateOpts) (string, error) {
	// Check for changes
	status, _ := git.ExecGitDir(worktreePath, "status", "--porcelain")
	if strings.TrimSpace(status) == "" {
		slog.Info("no changes", "repo", filepath.Base(worktreePath))
		return "", nil
	}

	branchName := "minion/" + taskID

	// Stage all changes
	if _, err := git.ExecGitDir(worktreePath, "add", "-A"); err != nil {
		return "", fmt.Errorf("staging changes: %w", err)
	}

	// Commit
	commitMsg := fmt.Sprintf("%s\n\nAutomated by %s (task: %s)\n\nCo-Authored-By: Claude <noreply@anthropic.com>", title, principalRepo, taskID)
	if _, err := git.ExecGitDir(worktreePath, "commit", "-m", commitMsg); err != nil {
		return "", fmt.Errorf("committing changes: %w", err)
	}

	// Push
	if _, err := git.ExecGitDir(worktreePath, "push", "--force-with-lease", "-u", "origin", branchName); err != nil {
		return "", fmt.Errorf("pushing branch: %w", err)
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
