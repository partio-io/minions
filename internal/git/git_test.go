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

func TestListCommitSubjects(t *testing.T) {
	repo := initRepo(t)
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "slice 1/2: work")
	gitT(t, repo, "commit", "-q", "--allow-empty", "-m", "minion:slice 1/2")

	subjects, err := ListCommitSubjects(repo, "main")
	if err != nil {
		t.Fatalf("ListCommitSubjects: %v", err)
	}
	want := []string{"minion:slice 1/2", "slice 1/2: work", "initial"}
	if len(subjects) != len(want) {
		t.Fatalf("subjects = %v; want %v", subjects, want)
	}
	for i := range want {
		if subjects[i] != want[i] {
			t.Errorf("subjects[%d] = %q; want %q", i, subjects[i], want[i])
		}
	}
}

func TestListCommitSubjects_ErrorsOnUnknownRef(t *testing.T) {
	repo := initRepo(t)
	if _, err := ListCommitSubjects(repo, "origin/minion/absent"); err == nil {
		t.Fatal("ListCommitSubjects: want error for unknown ref, got nil")
	}
}
