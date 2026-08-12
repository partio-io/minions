package git

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
)

// ExecGit runs a git command and returns trimmed stdout.
// Uses CombinedOutput so git error messages (from stderr) are included in errors.
func ExecGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s: %w", strings.Join(args, " "), strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ExecGitDir runs a git command with -C <dir> and returns trimmed stdout.
func ExecGitDir(dir string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", dir}, args...)
	return ExecGit(fullArgs...)
}

// CommitEmpty records an empty commit with the given message in dir. The
// slice loop uses it for marker commits that identify a completed slice
// without touching any files.
func CommitEmpty(dir, message string) error {
	_, err := ExecGitDir(dir, "commit", "--allow-empty", "-m", message)
	return err
}

// Push pushes branch to origin with --force-with-lease, so a re-run that
// rebuilt the branch can overwrite its own remote leftovers. The lease is
// refreshed first: a long-lived clone's remote-tracking ref can be stale
// (the branch was deleted or moved server-side), and a stale lease rejects
// every push with "stale info". After the refresh the lease guards only the
// fetch-to-push window — the right semantic for machine-owned minion
// branches, which a re-run must always be able to rebuild.
func Push(dir, branch string) error {
	if _, err := ExecGitDir(dir, "fetch", "-q", "origin", "+refs/heads/"+branch+":refs/remotes/origin/"+branch); err != nil {
		// Origin no longer has the branch; drop the stale tracking ref so
		// the lease expects absence instead of the recorded old tip.
		_, _ = ExecGitDir(dir, "update-ref", "-d", "refs/remotes/origin/"+branch)
	}
	_, err := ExecGitDir(dir, "push", "--force-with-lease", "-u", "origin", branch)
	return err
}

// RemoteBranchExists reports whether branch exists on origin, asking the
// remote directly so a stale local clone can never answer for it.
func RemoteBranchExists(dir, branch string) (bool, error) {
	_, err := ExecGitDir(dir, "ls-remote", "--exit-code", "--heads", "origin", branch)
	if err == nil {
		return true, nil
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) && exitErr.ExitCode() == 2 {
		return false, nil
	}
	return false, err
}

// FetchBranch updates origin/<branch> in dir from the remote.
func FetchBranch(dir, branch string) error {
	_, err := ExecGitDir(dir, "fetch", "-q", "origin", branch)
	return err
}

// ListCommitSubjectsRange returns the subjects of the commits ref has and base
// does not, in dir, newest first.
//
// The range is the point of this helper. Callers count the slice markers a run
// left on its own branch, and every marker an earlier run merged into base is
// still reachable from ref. Counting from ref alone therefore grows by six,
// nine, twelve markers as minion work lands, until the count passes any plan's
// slice total and every resume fails.
func ListCommitSubjectsRange(dir, base, ref string) ([]string, error) {
	out, err := ExecGitDir(dir, "log", "--format=%s", base+".."+ref)
	if err != nil {
		return nil, err
	}
	if out == "" {
		return nil, nil
	}
	return strings.Split(out, "\n"), nil
}

// OriginDefaultBranch returns the short name of origin's default branch (e.g.
// "main"), or "" if it cannot be determined. It prefers the locally recorded
// origin/HEAD symref and falls back to rediscovering it from the remote.
func OriginDefaultBranch(dir string) string {
	read := func() string {
		out, err := ExecGitDir(dir, "symbolic-ref", "--quiet", "--short", "refs/remotes/origin/HEAD")
		if err != nil {
			return ""
		}
		return strings.TrimPrefix(strings.TrimSpace(out), "origin/")
	}

	if b := read(); b != "" {
		return b
	}
	// origin/HEAD is not recorded locally (common on shallow CI clones); ask the
	// remote to (re)discover it, then re-read.
	if _, err := ExecGitDir(dir, "remote", "set-head", "origin", "--auto"); err == nil {
		if b := read(); b != "" {
			return b
		}
	}
	return ""
}
