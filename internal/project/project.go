package project

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Project represents the top-level project configuration from .minions/project.yaml.
type Project struct {
	Version     string      `yaml:"version"`
	Org         string      `yaml:"org"`
	Principal   RepoRef     `yaml:"principal"`
	Repos       []RepoEntry `yaml:"repos"`
	Credentials Credentials `yaml:"credentials"`
	Defaults    Defaults    `yaml:"defaults"`
}

// RepoRef identifies the principal repository.
type RepoRef struct {
	Name     string `yaml:"name"`
	FullName string `yaml:"full_name"`
}

// RepoEntry describes a target repository.
type RepoEntry struct {
	Name       string `yaml:"name"`
	FullName   string `yaml:"full_name"`
	BuildInfo  string `yaml:"build_info"`
	DocsRepo   bool   `yaml:"docs_repo,omitempty"`
	GHTokenEnv string `yaml:"gh_token_env,omitempty"`
}

// Credentials holds env var names for secrets.
type Credentials struct {
	AnthropicAPIKeyEnv string `yaml:"anthropic_api_key_env"`
	GHTokenEnv         string `yaml:"gh_token_env"`
}

// Defaults holds project-wide default values.
type Defaults struct {
	MaxTurns int      `yaml:"max_turns"`
	PRLabels []string `yaml:"pr_labels"`
}

// Load reads a project configuration from the given path.
func Load(path string) (*Project, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading project config %s: %w", path, err)
	}

	var p Project
	if err := yaml.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("parsing project config %s: %w", path, err)
	}

	return &p, nil
}

// Discover scans workspace subdirectories for .minions/project.yaml and returns the first found.
// Returns nil (not an error) if no project config exists.
func Discover(workspaceRoot string) *Project {
	entries, err := os.ReadDir(workspaceRoot)
	if err != nil {
		return nil
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(workspaceRoot, e.Name(), ".minions", "project.yaml")
		if _, err := os.Stat(path); err == nil {
			p, err := Load(path)
			if err == nil {
				return p
			}
		}
	}
	return nil
}

// RepoByName looks up a repo entry by its short name.
func (p *Project) RepoByName(short string) *RepoEntry {
	for i := range p.Repos {
		if p.Repos[i].Name == short {
			return &p.Repos[i]
		}
	}
	return nil
}

// FullName returns the full GitHub name for a repo short name.
// Falls back to org/short if not found in config.
func (p *Project) FullName(short string) string {
	if r := p.RepoByName(short); r != nil && r.FullName != "" {
		return r.FullName
	}
	return p.Org + "/" + short
}

// DocsRepo returns the repo entry marked as docs_repo, or nil.
func (p *Project) DocsRepo() *RepoEntry {
	for i := range p.Repos {
		if p.Repos[i].DocsRepo {
			return &p.Repos[i]
		}
	}
	return nil
}

// PrincipalFullName returns the full GitHub name of the principal repository.
func (p *Project) PrincipalFullName() string {
	if p.Principal.FullName != "" {
		return p.Principal.FullName
	}
	if p.Principal.Name != "" {
		return p.Org + "/" + p.Principal.Name
	}
	return ""
}

// AnthropicKeyEnv returns the env var name for the Anthropic API key.
func (p *Project) AnthropicKeyEnv() string {
	if p.Credentials.AnthropicAPIKeyEnv != "" {
		return p.Credentials.AnthropicAPIKeyEnv
	}
	return "ANTHROPIC_API_KEY"
}

// GHTokenEnv resolves the env var name for a repo's GitHub token, with per-repo override.
func (p *Project) GHTokenEnv(repoName string) string {
	if r := p.RepoByName(repoName); r != nil && r.GHTokenEnv != "" {
		return r.GHTokenEnv
	}
	if p.Credentials.GHTokenEnv != "" {
		return p.Credentials.GHTokenEnv
	}
	return "GH_TOKEN"
}

// BuildInfo returns the build info string for a repo, or empty string if not configured.
func (p *Project) BuildInfo(repoName string) string {
	if r := p.RepoByName(repoName); r != nil {
		return r.BuildInfo
	}
	return ""
}

// RepoNames returns the short names of all configured repos.
func (p *Project) RepoNames() []string {
	names := make([]string, len(p.Repos))
	for i, r := range p.Repos {
		names[i] = r.Name
	}
	return names
}
