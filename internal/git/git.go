package git

import (
	"fmt"
	"os/exec"
	"strings"
)

// ExecGit runs a git command and returns trimmed stdout.
func ExecGit(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// ExecGitDir runs a git command with -C <dir> and returns trimmed stdout.
func ExecGitDir(dir string, args ...string) (string, error) {
	fullArgs := append([]string{"-C", dir}, args...)
	return ExecGit(fullArgs...)
}
