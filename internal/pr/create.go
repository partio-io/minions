package pr

import (
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/partio-io/minions/internal/git"
)

// Create stages, commits, pushes, and creates a PR for a minion's work.
// Returns the PR URL or empty string if no changes.
func Create(worktreePath, repoFullName, taskID, title, description, why string, labels []string) (string, error) {
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
	commitMsg := fmt.Sprintf("%s\n\nAutomated by partio-io/minions (task: %s)\n\nCo-Authored-By: Claude <noreply@anthropic.com>", title, taskID)
	if _, err := git.ExecGitDir(worktreePath, "commit", "-m", commitMsg); err != nil {
		return "", fmt.Errorf("committing changes: %w", err)
	}

	// Push
	if _, err := git.ExecGitDir(worktreePath, "push", "-u", "origin", branchName); err != nil {
		return "", fmt.Errorf("pushing branch: %w", err)
	}

	// Build gh pr create command
	var prBody string
	if description != "" {
		var bodyBuilder strings.Builder
		bodyBuilder.WriteString("## Objective\n\n")
		bodyBuilder.WriteString(description)
		bodyBuilder.WriteString("\n\n")
		if why != "" {
			bodyBuilder.WriteString("## Why\n\n")
			bodyBuilder.WriteString(why)
			bodyBuilder.WriteString("\n\n")
		}
		bodyBuilder.WriteString("---\n\n")
		bodyBuilder.WriteString(fmt.Sprintf("Automated PR by [partio-io/minions](https://github.com/partio-io/minions) · Task: `%s`\n\n", taskID))
		bodyBuilder.WriteString("*Created by an unattended coding agent. Please review carefully.*")
		prBody = bodyBuilder.String()
	} else {
		prBody = fmt.Sprintf("Automated PR by [partio-io/minions](https://github.com/partio-io/minions) · Task: `%s`\n\n*Created by an unattended coding agent. Please review carefully.*", taskID)
	}

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
