package workspace

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/partio-io/minions/internal/project"
)

// EnsureRepos checks that all required repos exist in the workspace.
// If a repo is missing and gh is available, it clones it.
func EnsureRepos(proj *project.Project, workspaceRoot string, repoNames []string) error {
	for _, name := range repoNames {
		repoPath := filepath.Join(workspaceRoot, name)
		if _, err := os.Stat(repoPath); err == nil {
			slog.Debug("repo already present", "repo", name, "path", repoPath)
			continue
		}

		fullName := proj.FullName(name)
		slog.Info("cloning missing repo", "repo", fullName, "path", repoPath)

		cmd := exec.Command("gh", "repo", "clone", fullName, repoPath, "--", "--depth=1")
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return fmt.Errorf("cloning %s: %w", fullName, err)
		}
	}
	return nil
}
