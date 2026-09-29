package worktree

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

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

	// Unshallow if needed — CI checkouts are often shallow and worktree
	// branches need shared history with main for PR creation.
	if out, _ := git.ExecGitDir(repoPath, "rev-parse", "--is-shallow-repository"); strings.TrimSpace(out) == "true" {
		slog.Info("unshallowing repository for worktree compatibility", "repo", repoPath)
		if _, err := git.ExecGitDir(repoPath, "fetch", "--unshallow"); err != nil {
			slog.Warn("could not unshallow repository", "error", err)
		}
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

	// Base the worktree branch on the latest upstream default branch rather than
	// the local HEAD. The persistent CI checkouts are long-lived and their local
	// default branch drifts behind origin; branching from local HEAD would make
	// every minion branch (and its PR) start from stale history and conflict with
	// whatever merged upstream since. Fetching and branching from origin/<default>
	// never moves the checkout's local branches or touches its working tree, so it
	// is safe even when the checkout is shared across worktrees.
	base := "HEAD"
	if ref, err := latestOriginBase(repoPath); err != nil {
		slog.Warn("could not resolve latest origin base; branching from local HEAD", "repo", repoPath, "error", err)
	} else {
		base = ref
	}

	// Create the worktree with a new branch from the resolved base.
	if _, err := git.ExecGitDir(repoPath, "worktree", "add", wtPath, "-b", branchName, base); err != nil {
		return "", fmt.Errorf("creating worktree: %w", err)
	}

	if err := configureWorktree(wtPath); err != nil {
		return "", err
	}
	return wtPath, nil
}

// configureWorktree sets the git user in a fresh worktree so commits work in
// CI, and disables commit signing.
func configureWorktree(wtPath string) error {
	if _, err := git.ExecGitDir(wtPath, "config", "user.name", "minion[bot]"); err != nil {
		return fmt.Errorf("configuring git user.name: %w", err)
	}
	if _, err := git.ExecGitDir(wtPath, "config", "user.email", "minion[bot]@users.noreply.github.com"); err != nil {
		return fmt.Errorf("configuring git user.email: %w", err)
	}
	if _, err := git.ExecGitDir(wtPath, "config", "commit.gpgsign", "false"); err != nil {
		return fmt.Errorf("configuring commit.gpgsign: %w", err)
	}
	return nil
}

// CreateAtBranch creates a worktree for a repo + task combination checked out
// at the existing minion/<taskID> branch tip. Unlike Create it neither
// re-bases the branch on origin's default nor deletes it — the slice loop
// uses it so slice N+1 re-attaches to the branch carrying slice N's commits.
func CreateAtBranch(repoPath, taskID string) (string, error) {
	branchName := "minion/" + taskID
	wtPath := filepath.Join(repoPath, worktreeDir, taskID)

	if _, err := git.ExecGitDir(repoPath, "rev-parse", "--git-dir"); err != nil {
		return "", fmt.Errorf("%s is not a git repository: %w", repoPath, err)
	}

	if _, err := git.ExecGitDir(repoPath, "show-ref", "--verify", "refs/heads/"+branchName); err != nil {
		return "", fmt.Errorf("branch %s does not exist: %w", branchName, err)
	}

	// Remove stale worktree if it exists — the branch must not be checked out
	// anywhere else for the new worktree to attach to it.
	if _, err := os.Stat(wtPath); err == nil {
		slog.Debug("cleaning up stale worktree", "path", wtPath)
		_ = removeWorktree(repoPath, wtPath)
	}

	if _, err := git.ExecGitDir(repoPath, "worktree", "add", wtPath, branchName); err != nil {
		return "", fmt.Errorf("creating worktree at %s: %w", branchName, err)
	}

	if err := configureWorktree(wtPath); err != nil {
		return "", err
	}
	return wtPath, nil
}

// CreateFromOrigin creates a worktree for a repo + task combination checked
// out from origin's minion/<taskID> branch, fetching it first. The local
// branch is created or reset to the fetched tip (-B), so whatever a stale
// local clone recorded for it is never trusted. Resume uses it to continue a
// previously pushed build.
func CreateFromOrigin(repoPath, taskID string) (string, error) {
	branchName := "minion/" + taskID
	wtPath := filepath.Join(repoPath, worktreeDir, taskID)

	if _, err := git.ExecGitDir(repoPath, "rev-parse", "--git-dir"); err != nil {
		return "", fmt.Errorf("%s is not a git repository: %w", repoPath, err)
	}

	if err := git.FetchBranch(repoPath, branchName); err != nil {
		return "", fmt.Errorf("fetching origin/%s: %w", branchName, err)
	}

	// Remove stale worktree if it exists — the branch must not be checked out
	// anywhere else for -B to reset it.
	if _, err := os.Stat(wtPath); err == nil {
		slog.Debug("cleaning up stale worktree", "path", wtPath)
		_ = removeWorktree(repoPath, wtPath)
	}

	if _, err := git.ExecGitDir(repoPath, "worktree", "add", "-B", branchName, wtPath, "origin/"+branchName); err != nil {
		return "", fmt.Errorf("creating worktree from origin/%s: %w", branchName, err)
	}

	if err := configureWorktree(wtPath); err != nil {
		return "", err
	}
	return wtPath, nil
}

// latestOriginBase fetches origin's default branch and returns the ref the new
// worktree should branch from (e.g. "origin/main"), so minion branches are based
// on the latest upstream default branch instead of a stale local checkout. It
// returns an error when the repo has no usable origin default branch, letting the
// caller fall back to local HEAD.
func latestOriginBase(repoPath string) (string, error) {
	if _, err := git.ExecGitDir(repoPath, "remote", "get-url", "origin"); err != nil {
		return "", fmt.Errorf("no origin remote: %w", err)
	}

	branch := git.OriginDefaultBranch(repoPath)
	if branch == "" {
		return "", fmt.Errorf("could not determine origin default branch")
	}

	if _, err := git.ExecGitDir(repoPath, "fetch", "--quiet", "origin", branch); err != nil {
		return "", fmt.Errorf("fetching origin/%s: %w", branch, err)
	}

	ref := "origin/" + branch
	if _, err := git.ExecGitDir(repoPath, "rev-parse", "--verify", "--quiet", ref+"^{commit}"); err != nil {
		return "", fmt.Errorf("resolving %s: %w", ref, err)
	}
	return ref, nil
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
