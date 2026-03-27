package executor

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/partio-io/minions/internal/context"
	"github.com/partio-io/minions/internal/program"
	"github.com/partio-io/minions/internal/project"
	"github.com/partio-io/minions/internal/repoconfig"
)

// buildAgentPrompt constructs the prompt for a sub-agent execution.
func buildAgentPrompt(prog *program.Program, agent *program.AgentDef, planText, workspaceRoot string, proj *project.Project, pt *context.PhaseTracker) string {
	var b strings.Builder
	repos := prog.EffectiveTargetRepos(agent)

	// Header
	header := fmt.Sprintf("# Minion Task: %s\n\nYou are a coding agent executing a task autonomously. Complete the task in a single session without human interaction.\n\n", prog.Title)
	b.WriteString(header)
	pt.AddContext("template:header", header)

	// Plan context
	if planText != "" {
		planSection := "## Implementation Plan\n\nFollow this plan that was reviewed and approved:\n\n" + planText + "\n\n---\n\n"
		b.WriteString(planSection)
		pt.AddContext("plan", planText)
	}

	// Task description
	desc := fmt.Sprintf("## Task\n\n%s\n\n", prog.Description)
	b.WriteString(desc)
	pt.AddContext("description", prog.Description)

	// Agent-specific instructions
	if agent.Instructions != "" {
		inst := fmt.Sprintf("## Agent Instructions\n\n%s\n\n", agent.Instructions)
		b.WriteString(inst)
		pt.AddContext("agent_instructions:"+agent.Name, agent.Instructions)
	}

	// Target repos
	b.WriteString("## Target Repos\n\nYou are working in a multi-repo workspace:\n\n")
	for _, repo := range repos {
		info := resolveBuildInfo(repo, workspaceRoot, proj)
		fmt.Fprintf(&b, "- `%s/` — %s\n", repo, info)
	}
	if len(repos) > 1 {
		b.WriteString("\nYour working directory is the workspace root. Each repo is a subdirectory.\n\n")
	} else {
		b.WriteString("\nYour working directory is the repo root.\n\n")
	}

	// Acceptance criteria
	if len(prog.AcceptanceCriteria) > 0 {
		b.WriteString("## Acceptance Criteria\n\nYou MUST satisfy ALL of the following before finishing:\n\n")
		for _, c := range prog.AcceptanceCriteria {
			fmt.Fprintf(&b, "- %s\n", c)
		}
		b.WriteString("\n")
	}

	// CLAUDE.md files
	for _, repo := range repos {
		claudePath := filepath.Join(workspaceRoot, repo, "CLAUDE.md")
		data, err := os.ReadFile(claudePath)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		fmt.Fprintf(&b, "## %s/CLAUDE.md\n```\n%s\n```\n\n", repo, content)
		pt.AddContext("claude_md:"+repo, content)
	}

	// Context hints (from program level)
	if len(prog.ContextHints) > 0 {
		b.WriteString("## Pre-Read Context\n\n")
		for _, hint := range prog.ContextHints {
			content := readContextHint(workspaceRoot, hint)
			b.WriteString(content)
			pt.AddContext("context_hint:"+hint, content)
		}
	}

	// Execution instructions
	b.WriteString(`## Instructions

1. **Read first.** Before writing any code, read and understand the relevant existing code.
2. **Follow conventions.** Match existing code style, file organization, and test patterns.
3. **Implement incrementally.** Make small, focused changes.
4. **Run checks.** After implementation, run the appropriate checks for each modified repo.
5. **Fix failures.** If checks fail, read the error output carefully and fix the issue.
6. **Keep changes minimal.** Implement exactly what the task asks for — nothing more.
`)

	return b.String()
}

// readContextHint reads a file or directory listing for a context hint.
func readContextHint(workspaceRoot, hint string) string {
	hintPath := filepath.Join(workspaceRoot, hint)
	info, err := os.Stat(hintPath)
	if err != nil {
		return fmt.Sprintf("### %s\n*(file not found)*\n\n", hint)
	}
	if info.IsDir() {
		entries, err := os.ReadDir(hintPath)
		if err != nil {
			return fmt.Sprintf("### %s (directory listing)\n```\n(error reading directory)\n```\n\n", hint)
		}
		var listing strings.Builder
		for _, e := range entries {
			fmt.Fprintln(&listing, e.Name())
		}
		return fmt.Sprintf("### %s (directory listing)\n```\n%s```\n\n", hint, listing.String())
	}
	data, err := os.ReadFile(hintPath)
	if err != nil {
		return fmt.Sprintf("### %s\n*(error reading file)*\n\n", hint)
	}
	return fmt.Sprintf("### %s\n```\n%s\n```\n\n", hint, strings.TrimSpace(string(data)))
}

// resolveBuildInfo returns the build info for a repo.
func resolveBuildInfo(repo, workspaceRoot string, proj *project.Project) string {
	repoPath := filepath.Join(workspaceRoot, repo)
	rc := repoconfig.LoadOrDefault(repoPath)
	if rc.BuildInfo != "" {
		return rc.BuildInfo
	}
	if proj != nil {
		if info := proj.BuildInfo(repo); info != "" {
			return info
		}
	}
	return "Unknown repo type"
}
