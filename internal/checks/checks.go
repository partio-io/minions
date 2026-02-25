package checks

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
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
	if fileExists(filepath.Join(repoPath, "package.json")) {
		return RepoNode
	}
	if fileExists(filepath.Join(repoPath, "mint.json")) {
		return RepoDocs
	}
	return RepoUnknown
}

// Run executes the appropriate checks for the repo at the given path.
// Returns the combined check output and any error.
func Run(repoPath string) (string, error) {
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

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
