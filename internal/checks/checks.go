package checks

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/partio-io/minions/internal/repoconfig"
)

// RepoType identifies the type of repo for check selection.
type RepoType int

const (
	RepoUnknown RepoType = iota
	RepoGo
	RepoNode
	RepoDocs
)

// Detect determines the repo type by checking for build system files.
func Detect(repoPath string) RepoType {
	if fileExists(filepath.Join(repoPath, "Makefile")) && fileExists(filepath.Join(repoPath, "go.mod")) {
		return RepoGo
	}
	if fileExists(filepath.Join(repoPath, "mint.json")) {
		return RepoDocs
	}
	if fileExists(filepath.Join(repoPath, "package.json")) {
		return RepoNode
	}
	return RepoUnknown
}

// Run executes the appropriate checks for the repo at the given path.
// It first checks for .minions/repo.yaml config, then falls back to auto-detection.
// Returns the combined check output and any error.
func Run(repoPath string) (string, error) {
	rc := repoconfig.LoadOrDefault(repoPath)
	if len(rc.Checks) > 0 {
		return RunWithConfig(repoPath, rc)
	}

	repoName := filepath.Base(repoPath)

	switch Detect(repoPath) {
	case RepoGo:
		slog.Info("running Go checks", "repo", repoName)
		return runGo(repoPath)
	case RepoNode:
		slog.Info("running Node checks", "repo", repoName)
		return runNode(repoPath)
	case RepoDocs:
		slog.Info("running docs checks", "repo", repoName)
		return runDocs(repoPath)
	default:
		slog.Warn("unknown repo type, skipping checks", "repo", repoName)
		return fmt.Sprintf("SKIP: unknown repo type for %s", repoName), nil
	}
}

// RunWithConfig executes checks defined in a RepoConfig.
func RunWithConfig(repoPath string, cfg *repoconfig.RepoConfig) (string, error) {
	repoName := filepath.Base(repoPath)
	var allOutput strings.Builder

	for _, check := range cfg.Checks {
		slog.Info("running configured check", "repo", repoName, "check", check.Name)

		cmd := exec.Command("sh", "-c", check.Command)
		cmd.Dir = repoPath
		out, err := cmd.CombinedOutput()
		allOutput.Write(out)
		allOutput.WriteByte('\n')

		if err != nil {
			return allOutput.String(), fmt.Errorf("check %q failed: %w", check.Name, err)
		}
	}

	return allOutput.String(), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
