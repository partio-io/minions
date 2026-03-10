package git

import (
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
