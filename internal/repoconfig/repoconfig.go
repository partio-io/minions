package repoconfig

import (
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// RepoConfig holds per-repository configuration from .minions/repo.yaml.
type RepoConfig struct {
	Version             string     `yaml:"version"`
	BuildInfo           string     `yaml:"build_info"`
	Checks              []CheckDef `yaml:"checks"`
	DefaultContextHints []string   `yaml:"default_context_hints"`
	GHTokenEnv          string     `yaml:"gh_token_env,omitempty"`
	MaxTurns            int        `yaml:"max_turns,omitempty"`
}

// CheckDef defines a named check command.
type CheckDef struct {
	Name    string `yaml:"name"`
	Command string `yaml:"command"`
}

// Load reads repo config from .minions/repo.yaml in the given repo path.
func Load(repoPath string) (*RepoConfig, error) {
	path := filepath.Join(repoPath, ".minions", "repo.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var rc RepoConfig
	if err := yaml.Unmarshal(data, &rc); err != nil {
		return nil, err
	}
	return &rc, nil
}

// LoadOrDefault returns the repo config if found, or an empty default.
func LoadOrDefault(repoPath string) *RepoConfig {
	rc, err := Load(repoPath)
	if err != nil {
		return &RepoConfig{}
	}
	return rc
}
