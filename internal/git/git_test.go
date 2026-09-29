package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// gitT runs a git command in dir and fails the test on error.
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

// initRepo creates a repo with one commit and committer identity configured.
func initRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "repo")
	gitT(t, filepath.Dir(repo), "init", "-q", "-b", "main", repo)
	gitT(t, repo, "config", "user.name", "t")
	gitT(t, repo, "config", "user.email", "t@t")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", "-A")
	gitT(t, repo, "commit", "-q", "-m", "initial")
	return repo
}

func TestCommitEmpty(t *testing.T) {
	tests := []struct {
		name    string
		dir     func(t *testing.T) string
		message string
		wantErr bool
	}{
		{name: "records empty commit with message", dir: initRepo, message: "minion:slice 1/3", wantErr: false},
		{name: "errors outside a repository", dir: func(t *testing.T) string { return t.TempDir() }, message: "x", wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := tt.dir(t)
			err := CommitEmpty(dir, tt.message)
			if tt.wantErr {
				if err == nil {
					t.Fatal("CommitEmpty: want error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("CommitEmpty: %v", err)
			}
			if got := gitT(t, dir, "log", "-1", "--format=%s"); got != tt.message {
				t.Errorf("commit subject = %q; want %q", got, tt.message)
			}
			if files := gitT(t, dir, "diff-tree", "--no-commit-id", "--name-only", "-r", "HEAD"); files != "" {
				t.Errorf("marker commit touches files:\n%s", files)
			}
		})
	}
}

func TestPush(t *testing.T) {
	root := t.TempDir()
	origin := filepath.Join(root, "origin.git")
	repo := filepath.Join(root, "repo")
	gitT(t, root, "init", "--bare", "-b", "main", origin)
	gitT(t, root, "clone", "-q", origin, repo)
	gitT(t, repo, "config", "user.name", "t")
	gitT(t, repo, "config", "user.email", "t@t")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", "-A")
	gitT(t, repo, "commit", "-q", "-m", "initial")
	gitT(t, repo, "push", "-q", "origin", "main")
	gitT(t, repo, "checkout", "-q", "-b", "minion/task-1")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "slice work")

	if err := Push(repo, "minion/task-1"); err != nil {
		t.Fatalf("Push (new branch): %v", err)
	}
	local := gitT(t, repo, "rev-parse", "HEAD")
	if got := gitT(t, origin, "rev-parse", "refs/heads/minion/task-1"); got != local {
		t.Errorf("origin tip = %s; want %s", got, local)
	}

	// A second push after more commits advances the same remote branch.
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "more work")
	if err := Push(repo, "minion/task-1"); err != nil {
		t.Fatalf("Push (advance): %v", err)
	}
	local = gitT(t, repo, "rev-parse", "HEAD")
	if got := gitT(t, origin, "rev-parse", "refs/heads/minion/task-1"); got != local {
		t.Errorf("origin tip after second push = %s; want %s", got, local)
	}
}

func TestPush_ErrorsWithoutRemote(t *testing.T) {
	repo := initRepo(t)
	if err := Push(repo, "main"); err == nil {
		t.Fatal("Push: want error for repo without origin, got nil")
	}
}

// cloneWithOrigin builds a bare origin with one commit on main and returns a
// clone of it plus the origin path.
func cloneWithOrigin(t *testing.T) (repo, origin string) {
	t.Helper()
	root := t.TempDir()
	origin = filepath.Join(root, "origin.git")
	repo = filepath.Join(root, "repo")
	gitT(t, root, "init", "--bare", "-b", "main", origin)
	gitT(t, root, "clone", "-q", origin, repo)
	gitT(t, repo, "config", "user.name", "t")
	gitT(t, repo, "config", "user.email", "t@t")
	if err := os.WriteFile(filepath.Join(repo, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", "-A")
	gitT(t, repo, "commit", "-q", "-m", "initial")
	gitT(t, repo, "push", "-q", "origin", "main")
	return repo, origin
}

func TestRemoteBranchExists(t *testing.T) {
	repo, _ := cloneWithOrigin(t)

	exists, err := RemoteBranchExists(repo, "minion/task-1")
	if err != nil {
		t.Fatalf("RemoteBranchExists (absent): %v", err)
	}
	if exists {
		t.Error("RemoteBranchExists = true for a branch origin does not have")
	}

	gitT(t, repo, "push", "-q", "origin", "main:minion/task-1")
	exists, err = RemoteBranchExists(repo, "minion/task-1")
	if err != nil {
		t.Fatalf("RemoteBranchExists (present): %v", err)
	}
	if !exists {
		t.Error("RemoteBranchExists = false for a branch origin has")
	}
}

func TestRemoteBranchExists_ErrorsWithoutRemote(t *testing.T) {
	repo := initRepo(t)
	if _, err := RemoteBranchExists(repo, "minion/task-1"); err == nil {
		t.Fatal("RemoteBranchExists: want error for repo without origin, got nil")
	}
}

// TestFetchBranch verifies the fetched ref reflects origin even when another
// clone advanced the branch behind this clone's back — the resume path's
// only source of truth.
func TestFetchBranch(t *testing.T) {
	repo, origin := cloneWithOrigin(t)

	pusher := filepath.Join(t.TempDir(), "pusher")
	gitT(t, t.TempDir(), "clone", "-q", origin, pusher)
	gitT(t, pusher, "checkout", "-q", "-b", "minion/task-1")
	gitT(t, pusher, "commit", "-q", "--allow-empty", "-m", "minion:slice 1/2")
	gitT(t, pusher, "push", "-q", "origin", "minion/task-1")
	tip := gitT(t, pusher, "rev-parse", "HEAD")

	if err := FetchBranch(repo, "minion/task-1"); err != nil {
		t.Fatalf("FetchBranch: %v", err)
	}
	if got := gitT(t, repo, "rev-parse", "origin/minion/task-1"); got != tip {
		t.Errorf("origin/minion/task-1 after fetch = %s; want %s", got, tip)
	}
}

func TestFetchBranch_ErrorsWhenBranchMissing(t *testing.T) {
	repo, _ := cloneWithOrigin(t)
	if err := FetchBranch(repo, "minion/absent"); err == nil {
		t.Fatal("FetchBranch: want error for branch origin does not have, got nil")
	}
}

func TestListCommitSubjectsRange(t *testing.T) {
	repo := initRepo(t)
	gitT(t, repo, "checkout", "-q", "-b", "minion/task-1")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "slice 1/2: work")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "minion:slice 1/2")

	subjects, err := ListCommitSubjectsRange(repo, "main", "minion/task-1")
	if err != nil {
		t.Fatalf("ListCommitSubjectsRange: %v", err)
	}
	want := []string{"minion:slice 1/2", "slice 1/2: work"}
	if len(subjects) != len(want) {
		t.Fatalf("subjects = %v; want %v", subjects, want)
	}
	for i := range want {
		if subjects[i] != want[i] {
			t.Errorf("subjects[%d] = %q; want %q", i, subjects[i], want[i])
		}
	}
}

// TestListCommitSubjectsRange_ExcludesBase is the regression guard. Markers
// merged into the base stay reachable from the branch tip forever, so a count
// taken from the tip grows without bound and eventually breaks every resume.
func TestListCommitSubjectsRange_ExcludesBase(t *testing.T) {
	repo := initRepo(t)
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "minion:slice 1/2")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "minion:slice 2/2")
	gitT(t, repo, "checkout", "-q", "-b", "minion/task-2")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "minion:slice 1/3")

	subjects, err := ListCommitSubjectsRange(repo, "main", "minion/task-2")
	if err != nil {
		t.Fatalf("ListCommitSubjectsRange: %v", err)
	}
	want := []string{"minion:slice 1/3"}
	if len(subjects) != len(want) || subjects[0] != want[0] {
		t.Errorf("subjects = %v; want %v (base markers must not be counted)", subjects, want)
	}
}

func TestListCommitSubjectsRange_ErrorsOnUnknownRef(t *testing.T) {
	repo := initRepo(t)
	if _, err := ListCommitSubjectsRange(repo, "main", "origin/minion/absent"); err == nil {
		t.Fatal("ListCommitSubjectsRange: want error for unknown ref, got nil")
	}
}

// TestListCommitsRange covers the sibling of the subject listing: every
// commit of the range, newest first, with the paths it changed. An empty
// marker commit lists no path, and a commit base already has stays out.
func TestListCommitsRange(t *testing.T) {
	repo := initRepo(t)
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "minion:slice 2/2")
	gitT(t, repo, "checkout", "-q", "-b", "minion/task-1")
	if err := os.MkdirAll(filepath.Join(repo, "p"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"p/x.go", "p/y.go"} {
		if err := os.WriteFile(filepath.Join(repo, f), []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitT(t, repo, "add", "-A")
	gitT(t, repo, "commit", "-q", "-m", "slice 1/2: work")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "minion:slice 1/2")
	gitT(t, repo, "rm", "-q", "a.txt")
	gitT(t, repo, "commit", "-q", "-m", "slice 2/2: remove")

	commits, err := ListCommitsRange(repo, "main", "minion/task-1")
	if err != nil {
		t.Fatalf("ListCommitsRange: %v", err)
	}
	want := []Commit{
		{Subject: "slice 2/2: remove", Files: []string{"a.txt"}},
		{Subject: "minion:slice 1/2"},
		{Subject: "slice 1/2: work", Files: []string{"p/x.go", "p/y.go"}},
	}
	if len(commits) != len(want) {
		t.Fatalf("commits = %+v; want %d commits", commits, len(want))
	}
	for i := range want {
		got := commits[i]
		if got.Subject != want[i].Subject {
			t.Errorf("commits[%d].Subject = %q; want %q", i, got.Subject, want[i].Subject)
		}
		if len(got.Hash) != 40 {
			t.Errorf("commits[%d].Hash = %q; want a full hash", i, got.Hash)
		}
		if strings.Join(got.Files, ",") != strings.Join(want[i].Files, ",") {
			t.Errorf("commits[%d].Files = %v; want %v", i, got.Files, want[i].Files)
		}
	}
}

// TestListCommitsRange_NonASCIIPathIsVerbatim: git quotes non-ASCII paths by
// default ("caf\303\251.go"); the listing turns that off so a caller can
// pass the path back to git.
func TestListCommitsRange_NonASCIIPathIsVerbatim(t *testing.T) {
	repo := initRepo(t)
	gitT(t, repo, "checkout", "-q", "-b", "minion/task-1")
	if err := os.WriteFile(filepath.Join(repo, "café.go"), []byte("package p\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, repo, "add", "-A")
	gitT(t, repo, "commit", "-q", "-m", "work")

	commits, err := ListCommitsRange(repo, "main", "minion/task-1")
	if err != nil {
		t.Fatalf("ListCommitsRange: %v", err)
	}
	if len(commits) != 1 || strings.Join(commits[0].Files, ",") != "café.go" {
		t.Errorf("commits = %+v; want one commit with file café.go", commits)
	}
}

func TestListCommitsRange_EmptyRange(t *testing.T) {
	repo := initRepo(t)
	commits, err := ListCommitsRange(repo, "main", "main")
	if err != nil {
		t.Fatalf("ListCommitsRange: %v", err)
	}
	if len(commits) != 0 {
		t.Errorf("commits = %+v; want none for an empty range", commits)
	}
}

func TestListCommitsRange_ErrorsOnUnknownRef(t *testing.T) {
	repo := initRepo(t)
	if _, err := ListCommitsRange(repo, "main", "origin/minion/absent"); err == nil {
		t.Fatal("ListCommitsRange: want error for unknown ref, got nil")
	}
}

func TestListTreeFiles(t *testing.T) {
	repo := initRepo(t)
	if err := os.MkdirAll(filepath.Join(repo, "p", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"p/x.go", "p/sub/y.go"} {
		if err := os.WriteFile(filepath.Join(repo, f), []byte("package p\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitT(t, repo, "add", "-A")
	gitT(t, repo, "commit", "-q", "-m", "add p")

	files, err := ListTreeFiles(repo, "main", "p")
	if err != nil {
		t.Fatalf("ListTreeFiles: %v", err)
	}
	// Direct children only: the subdirectory lists as itself, not its files.
	if got := strings.Join(files, ","); got != "p/sub,p/x.go" {
		t.Errorf("files = %v; want [p/sub p/x.go]", files)
	}

	absent, err := ListTreeFiles(repo, "main", "nope")
	if err != nil {
		t.Fatalf("ListTreeFiles on absent path: %v", err)
	}
	if len(absent) != 0 {
		t.Errorf("files = %v; want none for an absent path", absent)
	}
}

func TestOriginDefaultBranch(t *testing.T) {
	repo, _ := cloneWithOrigin(t)
	if got := OriginDefaultBranch(repo); got != "main" {
		t.Errorf("OriginDefaultBranch = %q; want %q", got, "main")
	}
}

func TestOriginDefaultBranch_EmptyWithoutOrigin(t *testing.T) {
	repo := initRepo(t)
	if got := OriginDefaultBranch(repo); got != "" {
		t.Errorf("OriginDefaultBranch = %q; want %q for a repo without origin", got, "")
	}
}

// setupOriginClone builds a bare origin with one commit on main and returns
// (origin, clone).
func setupOriginClone(t *testing.T) (string, string) {
	t.Helper()
	root := t.TempDir()
	seed := filepath.Join(root, "seed")
	origin := filepath.Join(root, "origin.git")
	clone := filepath.Join(root, "clone")
	gitT(t, root, "init", "-q", "-b", "main", seed)
	gitT(t, seed, "config", "user.name", "t")
	gitT(t, seed, "config", "user.email", "t@t")
	if err := os.WriteFile(filepath.Join(seed, "a.txt"), []byte("one"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitT(t, seed, "add", "-A")
	gitT(t, seed, "commit", "-q", "-m", "initial")
	gitT(t, root, "clone", "-q", "--bare", seed, origin)
	gitT(t, root, "clone", "-q", origin, clone)
	gitT(t, clone, "config", "user.name", "t")
	gitT(t, clone, "config", "user.email", "t@t")
	return origin, clone
}

// TestPush_RecoversFromStaleLeaseAfterRemoteDelete reproduces the 2026-07-27
// staged-run failure: the clone once pushed the branch (remote-tracking ref
// recorded), the branch was later deleted on origin, and a fresh build wants
// to push the recreated branch. A bare --force-with-lease trusts the stale
// tracking ref and rejects with "stale info"; Push must refresh the lease
// and succeed.
func TestPush_RecoversFromStaleLeaseAfterRemoteDelete(t *testing.T) {
	origin, clone := setupOriginClone(t)
	gitT(t, clone, "checkout", "-q", "-b", "minion/task-1")
	gitT(t, clone, "commit", "-q", "--allow-empty", "-m", "old build")
	gitT(t, clone, "push", "-q", "-u", "origin", "minion/task-1")
	gitT(t, origin, "branch", "-D", "minion/task-1")

	gitT(t, clone, "commit", "-q", "--allow-empty", "-m", "new build")
	if err := Push(clone, "minion/task-1"); err != nil {
		t.Fatalf("Push after remote delete: %v", err)
	}
	want := gitT(t, clone, "rev-parse", "HEAD")
	if got := gitT(t, origin, "rev-parse", "refs/heads/minion/task-1"); got != want {
		t.Errorf("origin tip = %s; want %s", got, want)
	}
}

// TestPush_OverwritesMovedRemoteAfterRefresh documents the machine-owned
// semantic: a minion branch may be rebuilt even when origin's tip moved
// behind the clone's back — the refreshed lease guards only the
// fetch-to-push window.
func TestPush_OverwritesMovedRemoteAfterRefresh(t *testing.T) {
	origin, clone := setupOriginClone(t)
	gitT(t, clone, "checkout", "-q", "-b", "minion/task-1")
	gitT(t, clone, "commit", "-q", "--allow-empty", "-m", "our build")
	gitT(t, clone, "push", "-q", "-u", "origin", "minion/task-1")

	other := filepath.Join(t.TempDir(), "other")
	gitT(t, filepath.Dir(other), "clone", "-q", origin, other)
	gitT(t, other, "config", "user.name", "t")
	gitT(t, other, "config", "user.email", "t@t")
	gitT(t, other, "checkout", "-q", "minion/task-1")
	gitT(t, other, "commit", "-q", "--allow-empty", "-m", "their change")
	gitT(t, other, "push", "-q", "origin", "minion/task-1")

	gitT(t, clone, "commit", "-q", "--allow-empty", "-m", "rebuild")
	if err := Push(clone, "minion/task-1"); err != nil {
		t.Fatalf("Push after remote moved: %v", err)
	}
	want := gitT(t, clone, "rev-parse", "HEAD")
	if got := gitT(t, origin, "rev-parse", "refs/heads/minion/task-1"); got != want {
		t.Errorf("origin tip = %s; want our rebuilt tip %s", got, want)
	}
}
