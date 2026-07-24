package pr

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitP runs a git command in dir and fails the test on error.
func gitP(t *testing.T, dir string, args ...string) string {
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

// setupClone builds a bare origin with one commit on main and returns a clone
// whose origin/HEAD is recorded (as git clone does on real checkouts).
func setupClone(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	origin := filepath.Join(root, "origin.git")
	repo := filepath.Join(root, "repo")

	gitP(t, root, "init", "-q", "-b", "main", seed)
	gitP(t, seed, "config", "user.name", "t")
	gitP(t, seed, "config", "user.email", "t@t")
	if err := os.WriteFile(filepath.Join(seed, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitP(t, seed, "add", "-A")
	gitP(t, seed, "commit", "-q", "-m", "initial")
	gitP(t, root, "clone", "-q", "--bare", seed, origin)
	gitP(t, root, "clone", "-q", origin, repo)
	gitP(t, repo, "config", "user.name", "t")
	gitP(t, repo, "config", "user.email", "t@t")
	return repo
}

// TestAheadOfOriginDefault_FullyPushedBranch covers the slice loop's shape:
// every slice is pushed before the PR is created, so the branch has no
// unpushed commits — yet it is ahead of origin's default and must count as
// having changes.
func TestAheadOfOriginDefault_FullyPushedBranch(t *testing.T) {
	repo := setupClone(t)
	gitP(t, repo, "checkout", "-q", "-b", "minion/task-1")
	gitP(t, repo, "commit", "-q", "--allow-empty", "-m", "minion:slice 1/1")
	gitP(t, repo, "push", "-q", "-u", "origin", "minion/task-1")

	if !aheadOfOriginDefault(repo) {
		t.Fatal("aheadOfOriginDefault = false for a pushed branch with commits beyond origin's default; want true")
	}
}

func TestAheadOfOriginDefault_AtDefaultTip(t *testing.T) {
	repo := setupClone(t)
	if aheadOfOriginDefault(repo) {
		t.Fatal("aheadOfOriginDefault = true for a clone at origin's default tip; want false")
	}
}

// TestCreate_SkipsWhenNoChanges pins the no-op path the guard exists for: a
// clean checkout at origin's default tip creates no PR and returns no error.
func TestCreate_SkipsWhenNoChanges(t *testing.T) {
	repo := setupClone(t)
	url, err := Create(repo, "acme/api", "task-1", "title", "desc", "", nil, "acme/api", nil)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if url != "" {
		t.Fatalf("Create returned URL %q for a changeless checkout; want empty", url)
	}
}
