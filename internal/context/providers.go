package context

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// ProviderInput holds the input for context providers.
type ProviderInput struct {
	PRRepo        string
	PRNumber      string
	WorkspaceRoot string
	RepoShort     string // short repo name (e.g. "app" from "partio-io/app")
	PromptsDir    string // directory for per-repo prompt files
}

// ProviderDef describes a context provider in an agent definition.
type ProviderDef struct {
	Type string `yaml:"type"`
	Into string `yaml:"into"`
}

// RunProviders executes each provider and populates the vars map.
func RunProviders(providers []ProviderDef, input ProviderInput, vars map[string]string) error {
	for _, p := range providers {
		val, err := runProvider(p.Type, input)
		if err != nil {
			return fmt.Errorf("context provider %q: %w", p.Type, err)
		}
		vars[p.Into] = val
	}
	return nil
}

func runProvider(providerType string, input ProviderInput) (string, error) {
	switch providerType {
	case "gh-pr-title":
		return ghPRField(input.PRRepo, input.PRNumber, "title"), nil
	case "gh-pr-body":
		return ghPRField(input.PRRepo, input.PRNumber, "body"), nil
	case "gh-pr-diff":
		return ghPRDiff(input.PRRepo, input.PRNumber), nil
	case "repo-claude-md":
		return readFile(filepath.Join(input.WorkspaceRoot, input.RepoShort, "CLAUDE.md")), nil
	case "docs-claude-md":
		return readFile(filepath.Join(input.WorkspaceRoot, "docs", "CLAUDE.md")), nil
	case "current-readme":
		return readFile(filepath.Join(input.WorkspaceRoot, input.RepoShort, "README.md")), nil
	case "custom-prompt":
		path := filepath.Join(input.PromptsDir, "readme-"+input.RepoShort+".md")
		return readFile(path), nil
	default:
		return "", fmt.Errorf("unknown context provider: %s", providerType)
	}
}

func ghPRField(repo, number, field string) string {
	cmd := exec.Command("gh", "pr", "view", number, "--repo", repo, "--json", field, "-q", "."+field)
	out, err := cmd.Output()
	if err != nil {
		return "Unknown"
	}
	return strings.TrimSpace(string(out))
}

func ghPRDiff(repo, number string) string {
	cmd := exec.Command("gh", "pr", "diff", number, "--repo", repo)
	out, err := cmd.Output()
	if err != nil {
		return "Unable to fetch diff"
	}
	return string(out)
}

func readFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}
