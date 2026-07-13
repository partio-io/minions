package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// git runs a git command in dir and fails the test on error.
func gitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

func commitFile(t *testing.T, repo, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", "-A")
	gitT(t, repo, "commit", "-q", "-m", "add "+name)
}

// TestCreate_BranchesFromLatestOrigin verifies that when the local checkout's
// default branch has drifted behind origin, Create bases the new worktree on
// origin's tip (the bug: it used to branch from stale local HEAD).
func TestCreate_BranchesFromLatestOrigin(t *testing.T) {
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	local := filepath.Join(root, "local")
	pusher := filepath.Join(root, "pusher")

	// Bare origin with default branch "main".
	gitT(t, root, "init", "--bare", "-b", "main", origin)

	// Local checkout (the persistent CI clone) with one commit pushed to origin.
	gitT(t, root, "clone", "-q", origin, local)
	commitFile(t, local, "a.txt", "one")
	gitT(t, local, "push", "-q", "origin", "main")
	staleHead := gitT(t, local, "rev-parse", "HEAD")

	// A second clone advances origin/main without the local checkout knowing.
	gitT(t, root, "clone", "-q", origin, pusher)
	commitFile(t, pusher, "b.txt", "two")
	gitT(t, pusher, "push", "-q", "origin", "main")
	freshHead := gitT(t, pusher, "rev-parse", "HEAD")

	if staleHead == freshHead {
		t.Fatal("test setup: origin did not advance")
	}
	// Local HEAD is still the stale commit and it has not fetched.
	if got := gitT(t, local, "rev-parse", "HEAD"); got != staleHead {
		t.Fatalf("local HEAD moved unexpectedly: %s", got)
	}

	wtPath, err := Create(local, "task-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	gotBase := gitT(t, wtPath, "rev-parse", "HEAD")
	if gotBase != freshHead {
		t.Fatalf("worktree branched from %s; want latest origin %s (stale local was %s)", gotBase, freshHead, staleHead)
	}

	// The shared local checkout must be untouched: still on main at the stale commit.
	if got := gitT(t, local, "rev-parse", "main"); got != staleHead {
		t.Fatalf("local main was moved to %s; must stay at %s", got, staleHead)
	}
}

// TestCreate_FallsBackToHEADWithoutOrigin verifies graceful degradation: a repo
// with no origin remote still gets a worktree, branched from local HEAD.
func TestCreate_FallsBackToHEADWithoutOrigin(t *testing.T) {
	root := t.TempDir()
	repo := filepath.Join(root, "repo")
	gitT(t, root, "init", "-b", "main", repo)
	commitFile(t, repo, "a.txt", "one")
	head := gitT(t, repo, "rev-parse", "HEAD")

	wtPath, err := Create(repo, "task-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got := gitT(t, wtPath, "rev-parse", "HEAD"); got != head {
		t.Fatalf("worktree HEAD = %s; want local HEAD %s", got, head)
	}
}
