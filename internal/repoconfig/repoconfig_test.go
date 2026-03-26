package repoconfig

import (
	"os"
	"path/filepath"
	"testing"
)

const testRepoYAML = `version: "1"
build_info: "Go API (build: make test)"
checks:
  - name: lint
    command: "make lint"
  - name: test
    command: "make test"
default_context_hints:
  - "CLAUDE.md"
  - "internal/"
gh_token_env: GH_TOKEN_API
max_turns: 50
`

func TestLoad(t *testing.T) {
	dir := t.TempDir()
	minionsDir := filepath.Join(dir, ".minions")
	os.MkdirAll(minionsDir, 0755)
	os.WriteFile(filepath.Join(minionsDir, "repo.yaml"), []byte(testRepoYAML), 0644)

	rc, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	if rc.BuildInfo != "Go API (build: make test)" {
		t.Errorf("BuildInfo = %q", rc.BuildInfo)
	}
	if len(rc.Checks) != 2 {
		t.Fatalf("len(Checks) = %d, want 2", len(rc.Checks))
	}
	if rc.Checks[0].Name != "lint" || rc.Checks[0].Command != "make lint" {
		t.Errorf("Checks[0] = %+v", rc.Checks[0])
	}
	if len(rc.DefaultContextHints) != 2 {
		t.Errorf("len(DefaultContextHints) = %d, want 2", len(rc.DefaultContextHints))
	}
	if rc.GHTokenEnv != "GH_TOKEN_API" {
		t.Errorf("GHTokenEnv = %q", rc.GHTokenEnv)
	}
	if rc.MaxTurns != 50 {
		t.Errorf("MaxTurns = %d, want 50", rc.MaxTurns)
	}
}

func TestLoadOrDefault_Missing(t *testing.T) {
	dir := t.TempDir()
	rc := LoadOrDefault(dir)

	if rc.BuildInfo != "" {
		t.Errorf("BuildInfo = %q, want empty", rc.BuildInfo)
	}
	if len(rc.Checks) != 0 {
		t.Errorf("len(Checks) = %d, want 0", len(rc.Checks))
	}
}

func TestLoadOrDefault_Present(t *testing.T) {
	dir := t.TempDir()
	minionsDir := filepath.Join(dir, ".minions")
	os.MkdirAll(minionsDir, 0755)
	os.WriteFile(filepath.Join(minionsDir, "repo.yaml"), []byte(testRepoYAML), 0644)

	rc := LoadOrDefault(dir)
	if rc.BuildInfo == "" {
		t.Error("BuildInfo should not be empty when repo.yaml exists")
	}
}
