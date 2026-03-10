package agent

import (
	"testing"
)

func TestLoadBuiltinAgents(t *testing.T) {
	agents := []string{"task-runner", "doc-updater", "readme-updater"}
	for _, name := range agents {
		def, err := Load(name)
		if err != nil {
			t.Fatalf("Load(%q): %v", name, err)
		}
		if def.Name != name {
			t.Errorf("Load(%q).Name = %q, want %q", name, def.Name, name)
		}
		if def.PromptTemplate == "" {
			t.Errorf("Load(%q).PromptTemplate is empty", name)
		}
		if def.MaxTurns == 0 {
			t.Errorf("Load(%q).MaxTurns is 0", name)
		}
	}
}

func TestLoadNotFound(t *testing.T) {
	_, err := Load("nonexistent")
	if err == nil {
		t.Fatal("Load(nonexistent) should return error")
	}
}

func TestList(t *testing.T) {
	names, err := List()
	if err != nil {
		t.Fatalf("List(): %v", err)
	}
	if len(names) < 3 {
		t.Errorf("List() returned %d agents, want at least 3", len(names))
	}

	expected := map[string]bool{"task-runner": false, "doc-updater": false, "readme-updater": false}
	for _, n := range names {
		expected[n] = true
	}
	for name, found := range expected {
		if !found {
			t.Errorf("List() missing %q", name)
		}
	}
}

func TestRenderTemplate(t *testing.T) {
	tmpl := "Hello {{NAME}}, you have {{COUNT}} items."
	vars := map[string]string{
		"NAME":  "World",
		"COUNT": "42",
	}
	got := RenderTemplate(tmpl, vars)
	want := "Hello World, you have 42 items."
	if got != want {
		t.Errorf("RenderTemplate() = %q, want %q", got, want)
	}
}

func TestRenderTemplateUnusedVars(t *testing.T) {
	tmpl := "Hello {{NAME}}"
	vars := map[string]string{
		"NAME":  "World",
		"EXTRA": "unused",
	}
	got := RenderTemplate(tmpl, vars)
	want := "Hello World"
	if got != want {
		t.Errorf("RenderTemplate() = %q, want %q", got, want)
	}
}

func TestRenderTemplateMissingVars(t *testing.T) {
	tmpl := "Hello {{NAME}}, count: {{COUNT}}"
	vars := map[string]string{
		"NAME": "World",
	}
	got := RenderTemplate(tmpl, vars)
	want := "Hello World, count: {{COUNT}}"
	if got != want {
		t.Errorf("RenderTemplate() = %q, want %q", got, want)
	}
}

func TestDocUpdaterHasContextProviders(t *testing.T) {
	def, err := Load("doc-updater")
	if err != nil {
		t.Fatalf("Load(doc-updater): %v", err)
	}
	if len(def.ContextProviders) == 0 {
		t.Error("doc-updater should have context providers")
	}

	providerTypes := make(map[string]bool)
	for _, p := range def.ContextProviders {
		providerTypes[p.Type] = true
	}

	for _, expected := range []string{"gh-pr-title", "gh-pr-body", "gh-pr-diff"} {
		if !providerTypes[expected] {
			t.Errorf("doc-updater missing context provider %q", expected)
		}
	}
}

func TestReadmeUpdaterConfig(t *testing.T) {
	def, err := Load("readme-updater")
	if err != nil {
		t.Fatalf("Load(readme-updater): %v", err)
	}
	if def.RunChecks {
		t.Error("readme-updater should not run checks")
	}
	if def.SkipMarker != ".no-update-needed" {
		t.Errorf("readme-updater.SkipMarker = %q, want %q", def.SkipMarker, ".no-update-needed")
	}
	if len(def.StageFiles) != 1 || def.StageFiles[0] != "README.md" {
		t.Errorf("readme-updater.StageFiles = %v, want [README.md]", def.StageFiles)
	}
}
