package project

import (
	"os"
	"path/filepath"
	"testing"
)

const testProjectYAML = `version: "1"
org: test-org

principal:
  name: minions
  full_name: test-org/minions

repos:
  - name: api
    full_name: test-org/api
    build_info: "Go API"
    gh_token_env: GH_TOKEN_API

  - name: web
    full_name: test-org/web
    build_info: "Next.js app"

  - name: docs
    full_name: test-org/docs
    build_info: "Documentation site"
    docs_repo: true

credentials:
  anthropic_api_key_env: MY_CLAUDE_KEY
  gh_token_env: MY_GH_TOKEN

defaults:
  max_turns: 20
  pr_labels:
    - bot
`

func writeTestProject(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "project.yaml")
	if err := os.WriteFile(path, []byte(testProjectYAML), 0644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad(t *testing.T) {
	path := writeTestProject(t)
	p, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}

	if p.Org != "test-org" {
		t.Errorf("Org = %q, want %q", p.Org, "test-org")
	}
	if p.Principal.FullName != "test-org/minions" {
		t.Errorf("Principal.FullName = %q, want %q", p.Principal.FullName, "test-org/minions")
	}
	if len(p.Repos) != 3 {
		t.Errorf("len(Repos) = %d, want 3", len(p.Repos))
	}
}

func TestRepoByName(t *testing.T) {
	path := writeTestProject(t)
	p, _ := Load(path)

	r := p.RepoByName("api")
	if r == nil {
		t.Fatal("RepoByName(api) returned nil")
	}
	if r.FullName != "test-org/api" {
		t.Errorf("FullName = %q, want %q", r.FullName, "test-org/api")
	}

	if p.RepoByName("nonexistent") != nil {
		t.Error("RepoByName(nonexistent) should return nil")
	}
}

func TestFullName(t *testing.T) {
	path := writeTestProject(t)
	p, _ := Load(path)

	if got := p.FullName("api"); got != "test-org/api" {
		t.Errorf("FullName(api) = %q, want %q", got, "test-org/api")
	}
	// Unknown repo falls back to org/name
	if got := p.FullName("unknown"); got != "test-org/unknown" {
		t.Errorf("FullName(unknown) = %q, want %q", got, "test-org/unknown")
	}
}

func TestDocsRepo(t *testing.T) {
	path := writeTestProject(t)
	p, _ := Load(path)

	dr := p.DocsRepo()
	if dr == nil {
		t.Fatal("DocsRepo() returned nil")
	}
	if dr.Name != "docs" {
		t.Errorf("DocsRepo().Name = %q, want %q", dr.Name, "docs")
	}
}

func TestPrincipalFullName(t *testing.T) {
	path := writeTestProject(t)
	p, _ := Load(path)

	if got := p.PrincipalFullName(); got != "test-org/minions" {
		t.Errorf("PrincipalFullName() = %q, want %q", got, "test-org/minions")
	}
}

func TestAnthropicKeyEnv(t *testing.T) {
	path := writeTestProject(t)
	p, _ := Load(path)

	if got := p.AnthropicKeyEnv(); got != "MY_CLAUDE_KEY" {
		t.Errorf("AnthropicKeyEnv() = %q, want %q", got, "MY_CLAUDE_KEY")
	}

	// Default when not configured
	p.Credentials.AnthropicAPIKeyEnv = ""
	if got := p.AnthropicKeyEnv(); got != "ANTHROPIC_API_KEY" {
		t.Errorf("AnthropicKeyEnv() = %q, want %q", got, "ANTHROPIC_API_KEY")
	}
}

func TestGHTokenEnv(t *testing.T) {
	path := writeTestProject(t)
	p, _ := Load(path)

	// Per-repo override
	if got := p.GHTokenEnv("api"); got != "GH_TOKEN_API" {
		t.Errorf("GHTokenEnv(api) = %q, want %q", got, "GH_TOKEN_API")
	}

	// Project default
	if got := p.GHTokenEnv("web"); got != "MY_GH_TOKEN" {
		t.Errorf("GHTokenEnv(web) = %q, want %q", got, "MY_GH_TOKEN")
	}

	// Unknown repo uses project default
	if got := p.GHTokenEnv("unknown"); got != "MY_GH_TOKEN" {
		t.Errorf("GHTokenEnv(unknown) = %q, want %q", got, "MY_GH_TOKEN")
	}
}

func TestDiscover(t *testing.T) {
	// Create a workspace with a repo containing .minions/project.yaml
	wsDir := t.TempDir()
	repoDir := filepath.Join(wsDir, "myrepo", ".minions")
	os.MkdirAll(repoDir, 0755)
	os.WriteFile(filepath.Join(repoDir, "project.yaml"), []byte(testProjectYAML), 0644)

	p := Discover(wsDir)
	if p == nil {
		t.Fatal("Discover returned nil")
	}
	if p.Org != "test-org" {
		t.Errorf("Org = %q, want %q", p.Org, "test-org")
	}

	// Empty workspace
	emptyDir := t.TempDir()
	if Discover(emptyDir) != nil {
		t.Error("Discover on empty dir should return nil")
	}
}

func TestBuildInfo(t *testing.T) {
	path := writeTestProject(t)
	p, _ := Load(path)

	if got := p.BuildInfo("api"); got != "Go API" {
		t.Errorf("BuildInfo(api) = %q, want %q", got, "Go API")
	}
	if got := p.BuildInfo("unknown"); got != "" {
		t.Errorf("BuildInfo(unknown) = %q, want empty", got)
	}
}

func TestRepoNames(t *testing.T) {
	path := writeTestProject(t)
	p, _ := Load(path)

	names := p.RepoNames()
	if len(names) != 3 {
		t.Fatalf("len(RepoNames()) = %d, want 3", len(names))
	}
	if names[0] != "api" || names[1] != "web" || names[2] != "docs" {
		t.Errorf("RepoNames() = %v, want [api web docs]", names)
	}
}
