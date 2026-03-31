package program

import (
	"testing"
)

const fullProgram = `---
id: detect-external-hooks
target_repos:
  - cli
  - docs
pr_labels:
  - minion
  - feature
acceptance_criteria:
  - "partio enable warns when Husky .husky/ directory is detected"
  - "make test passes in cli/"
---

# Detect external hook managers during enable

During ` + "`partio enable`" + `, detect if external Git hook managers are installed.
Warn the user about potential conflicts.

## Context

- cli/cmd/partio/enable.go
- cli/internal/git/hooks/
- docs/cli/commands.mdx

## Planner

` + "```capabilities" + `
tools: Read, Glob, Grep, Bash
permission_mode: plan
max_turns: 15
` + "```" + `

Explore how partio enable currently works.

## Agents

### cli-agent

` + "```capabilities" + `
tools: Edit, Write, Read, Glob, Grep, Bash
target_repos:
  - cli
max_turns: 30
checks: true
retry_on_fail: true
` + "```" + `

Implement detection logic in the CLI.

### docs-agent

` + "```capabilities" + `
tools: Edit, Write, Read
target_repos:
  - docs
max_turns: 15
checks: true
` + "```" + `

Update docs to reflect the new behavior.
`

func TestParseFullProgram(t *testing.T) {
	p, err := Parse(fullProgram)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	// Frontmatter
	if p.ID != "detect-external-hooks" {
		t.Errorf("ID = %q, want %q", p.ID, "detect-external-hooks")
	}
	if len(p.TargetRepos) != 2 || p.TargetRepos[0] != "cli" || p.TargetRepos[1] != "docs" {
		t.Errorf("TargetRepos = %v, want [cli docs]", p.TargetRepos)
	}
	if len(p.PRLabels) != 2 {
		t.Errorf("PRLabels = %v, want 2 items", p.PRLabels)
	}
	if len(p.AcceptanceCriteria) != 2 {
		t.Errorf("AcceptanceCriteria = %v, want 2 items", p.AcceptanceCriteria)
	}

	// Title & Description
	if p.Title != "Detect external hook managers during enable" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.Description == "" {
		t.Error("Description is empty")
	}

	// Context
	if len(p.ContextHints) != 3 {
		t.Errorf("ContextHints = %v, want 3 items", p.ContextHints)
	}
	if p.ContextHints[0] != "cli/cmd/partio/enable.go" {
		t.Errorf("ContextHints[0] = %q", p.ContextHints[0])
	}

	// Planner
	if p.Planner == nil {
		t.Fatal("Planner is nil")
	}
	if len(p.Planner.Tools) != 4 {
		t.Errorf("Planner.Tools = %v, want 4 items", p.Planner.Tools)
	}
	if p.Planner.PermissionMode != "plan" {
		t.Errorf("Planner.PermissionMode = %q", p.Planner.PermissionMode)
	}
	if p.Planner.MaxTurns != 15 {
		t.Errorf("Planner.MaxTurns = %d", p.Planner.MaxTurns)
	}
	if p.Planner.Instructions == "" {
		t.Error("Planner.Instructions is empty")
	}

	// Agents
	if len(p.Agents) != 2 {
		t.Fatalf("Agents = %d, want 2", len(p.Agents))
	}

	cli := p.Agents[0]
	if cli.Name != "cli-agent" {
		t.Errorf("Agents[0].Name = %q", cli.Name)
	}
	if len(cli.Tools) != 6 {
		t.Errorf("Agents[0].Tools = %v, want 6 items", cli.Tools)
	}
	if len(cli.TargetRepos) != 1 || cli.TargetRepos[0] != "cli" {
		t.Errorf("Agents[0].TargetRepos = %v", cli.TargetRepos)
	}
	if cli.MaxTurns != 30 {
		t.Errorf("Agents[0].MaxTurns = %d", cli.MaxTurns)
	}
	if !cli.Checks {
		t.Error("Agents[0].Checks should be true")
	}
	if !cli.RetryOnFail {
		t.Error("Agents[0].RetryOnFail should be true")
	}
	if cli.Instructions == "" {
		t.Error("Agents[0].Instructions is empty")
	}

	docs := p.Agents[1]
	if docs.Name != "docs-agent" {
		t.Errorf("Agents[1].Name = %q", docs.Name)
	}
	if len(docs.Tools) != 3 {
		t.Errorf("Agents[1].Tools = %v, want 3 items", docs.Tools)
	}
	if len(docs.TargetRepos) != 1 || docs.TargetRepos[0] != "docs" {
		t.Errorf("Agents[1].TargetRepos = %v", docs.TargetRepos)
	}
}

func TestParseNoPlanner(t *testing.T) {
	content := `---
id: simple-task
target_repos:
  - cli
---

# Simple task

Do something simple.

## Agents

### the-agent

` + "```capabilities" + `
tools: Edit, Read
max_turns: 10
` + "```" + `

Just do the thing.
`
	p, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if p.ID != "simple-task" {
		t.Errorf("ID = %q", p.ID)
	}
	if p.Planner != nil {
		t.Error("Planner should be nil")
	}
	if len(p.Agents) != 1 {
		t.Fatalf("Agents = %d, want 1", len(p.Agents))
	}
	if p.Agents[0].Name != "the-agent" {
		t.Errorf("Agent name = %q", p.Agents[0].Name)
	}
}

func TestParseNoAgents(t *testing.T) {
	content := `---
id: implicit-agent
target_repos:
  - app
---

# Implicit agent task

This is a task with no explicit agents section.
The description becomes the implicit agent instructions.
`
	p, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if p.ID != "implicit-agent" {
		t.Errorf("ID = %q", p.ID)
	}
	if len(p.Agents) != 0 {
		t.Errorf("Agents = %d, want 0", len(p.Agents))
	}
	if p.Description == "" {
		t.Error("Description should not be empty")
	}
}

func TestParseNoFrontmatter(t *testing.T) {
	content := `# Just a title

Some description.
`
	p, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if p.Title != "Just a title" {
		t.Errorf("Title = %q", p.Title)
	}
	if p.ID != "" {
		t.Errorf("ID = %q, want empty", p.ID)
	}
}

func TestParseToolsList(t *testing.T) {
	content := `---
id: tools-list-test
---

# Tools list test

## Agents

### agent-a

` + "```capabilities" + `
tools:
  - Edit
  - Read
  - Bash
max_turns: 5
` + "```" + `

Test YAML list syntax for tools.
`
	p, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(p.Agents) != 1 {
		t.Fatalf("Agents = %d, want 1", len(p.Agents))
	}
	if len(p.Agents[0].Tools) != 3 {
		t.Errorf("Tools = %v, want 3 items", p.Agents[0].Tools)
	}
	if p.Agents[0].Tools[0] != "Edit" {
		t.Errorf("Tools[0] = %q", p.Agents[0].Tools[0])
	}
}

func TestParseMCPs(t *testing.T) {
	content := `---
id: mcp-test
---

# MCP test

## Agents

### mcp-agent

` + "```capabilities" + `
tools: Read, Bash
max_turns: 10
mcps:
  - name: context7
    type: stdio
    command: npx
    args: ["-y", "@anthropic-ai/context7-mcp"]
  - name: remote-api
    type: http
    url: https://api.example.com/mcp
    headers:
      Authorization: "Bearer token"
` + "```" + `

Use MCPs to do stuff.
`
	p, err := Parse(content)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}

	if len(p.Agents) != 1 {
		t.Fatalf("Agents = %d, want 1", len(p.Agents))
	}
	agent := p.Agents[0]
	if len(agent.MCPs) != 2 {
		t.Fatalf("MCPs = %d, want 2", len(agent.MCPs))
	}

	mcp0 := agent.MCPs[0]
	if mcp0.Name != "context7" || mcp0.Type != "stdio" || mcp0.Command != "npx" {
		t.Errorf("MCP[0] = %+v", mcp0)
	}
	if len(mcp0.Args) != 2 {
		t.Errorf("MCP[0].Args = %v", mcp0.Args)
	}

	mcp1 := agent.MCPs[1]
	if mcp1.Name != "remote-api" || mcp1.Type != "http" || mcp1.URL != "https://api.example.com/mcp" {
		t.Errorf("MCP[1] = %+v", mcp1)
	}
	if mcp1.Headers["Authorization"] != "Bearer token" {
		t.Errorf("MCP[1].Headers = %v", mcp1.Headers)
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name    string
		program Program
		wantErr bool
	}{
		{
			name:    "valid",
			program: Program{ID: "test", Title: "Test"},
			wantErr: false,
		},
		{
			name:    "missing id",
			program: Program{Title: "Test"},
			wantErr: true,
		},
		{
			name:    "missing title",
			program: Program{ID: "test"},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.program.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAllTargetRepos(t *testing.T) {
	p := &Program{
		TargetRepos: []string{"cli", "docs"},
		Agents: []AgentDef{
			{Name: "a", TargetRepos: []string{"cli"}},
			{Name: "b", TargetRepos: []string{"app"}},
		},
	}

	repos := p.AllTargetRepos()
	if len(repos) != 3 {
		t.Errorf("AllTargetRepos() = %v, want 3 items", repos)
	}
}

func TestEffectiveTargetRepos(t *testing.T) {
	p := &Program{
		TargetRepos: []string{"cli", "docs"},
	}

	// Agent with override
	a := &AgentDef{TargetRepos: []string{"app"}}
	repos := p.EffectiveTargetRepos(a)
	if len(repos) != 1 || repos[0] != "app" {
		t.Errorf("EffectiveTargetRepos(override) = %v", repos)
	}

	// Agent without override
	b := &AgentDef{}
	repos = p.EffectiveTargetRepos(b)
	if len(repos) != 2 || repos[0] != "cli" {
		t.Errorf("EffectiveTargetRepos(fallback) = %v", repos)
	}
}
