package worktree

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/partio-io/minions/internal/git"
)

const worktreeDir = ".minion-worktrees"

// Create creates a git worktree for a repo + task combination.
// Returns the path to the created worktree.
func Create(repoPath, taskID string) (string, error) {
	branchName := "minion/" + taskID
	wtPath := filepath.Join(repoPath, worktreeDir, taskID)

	// Verify it's a git repo
	if _, err := git.ExecGitDir(repoPath, "rev-parse", "--git-dir"); err != nil {
		return "", fmt.Errorf("%s is not a git repository: %w", repoPath, err)
	}

	// Remove stale worktree if it exists
	if _, err := os.Stat(wtPath); err == nil {
		slog.Debug("cleaning up stale worktree", "path", wtPath)
		_ = removeWorktree(repoPath, wtPath)
	}

	// Delete branch if it already exists
	if out, _ := git.ExecGitDir(repoPath, "show-ref", "--verify", "refs/heads/"+branchName); out != "" {
		_, _ = git.ExecGitDir(repoPath, "branch", "-D", branchName)
	}

	// Create the worktree with a new branch from HEAD
	if _, err := git.ExecGitDir(repoPath, "worktree", "add", wtPath, "-b", branchName, "HEAD"); err != nil {
		return "", fmt.Errorf("creating worktree: %w", err)
	}

	// Configure git user in worktree so commits work in CI
	if _, err := git.ExecGitDir(wtPath, "config", "user.name", "minion[bot]"); err != nil {
		return "", fmt.Errorf("configuring git user.name: %w", err)
	}
	if _, err := git.ExecGitDir(wtPath, "config", "user.email", "minion[bot]@users.noreply.github.com"); err != nil {
		return "", fmt.Errorf("configuring git user.email: %w", err)
	}
	if _, err := git.ExecGitDir(wtPath, "config", "commit.gpgsign", "false"); err != nil {
		return "", fmt.Errorf("configuring commit.gpgsign: %w", err)
	}

	return wtPath, nil
}

// Cleanup removes a worktree created by Create.
func Cleanup(repoPath, taskID string) {
	wtPath := filepath.Join(repoPath, worktreeDir, taskID)
	if _, err := os.Stat(wtPath); err == nil {
		_ = removeWorktree(repoPath, wtPath)
	}
}

// CleanupAll removes all minion worktrees for a repo.
func CleanupAll(repoPath string) {
	wtBase := filepath.Join(repoPath, worktreeDir)
	entries, err := os.ReadDir(wtBase)
	if err != nil {
		return
	}

	for _, e := range entries {
		if e.IsDir() {
			wtPath := filepath.Join(wtBase, e.Name())
			_ = removeWorktree(repoPath, wtPath)
		}
	}

	_ = os.Remove(wtBase)
	_, _ = git.ExecGitDir(repoPath, "worktree", "prune")
}

func removeWorktree(repoPath, wtPath string) error {
	_, err := git.ExecGitDir(repoPath, "worktree", "remove", "--force", wtPath)
	return err
}
