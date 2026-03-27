package planner

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

// buildPrompt constructs the planner prompt and records context metrics.
func buildPrompt(prog *program.Program, workspaceRoot string, proj *project.Project, pt *context.PhaseTracker) string {
	var b strings.Builder

	// Header
	header := fmt.Sprintf("# Minion Planning: %s\n\nYou are a coding agent planning an implementation. Explore the codebase thoroughly and produce a detailed plan. Do NOT make any changes — this is a read-only planning phase.\n\n", prog.Title)
	b.WriteString(header)
	pt.AddContext("template:header", header)

	// Task description
	desc := fmt.Sprintf("## Task\n\n%s\n\n", prog.Description)
	b.WriteString(desc)
	pt.AddContext("description", prog.Description)

	// Planner instructions
	if prog.Planner != nil && prog.Planner.Instructions != "" {
		inst := fmt.Sprintf("## Planner Instructions\n\n%s\n\n", prog.Planner.Instructions)
		b.WriteString(inst)
		pt.AddContext("planner_instructions", prog.Planner.Instructions)
	}

	// Target repos
	b.WriteString("## Target Repos\n\nYou are working in a multi-repo workspace. The following repos are checked out side-by-side:\n\n")
	for _, repo := range prog.TargetRepos {
		info := resolveBuildInfo(repo, workspaceRoot, proj)
		fmt.Fprintf(&b, "- `%s/` — %s\n", repo, info)
	}
	b.WriteString("\nYour working directory is the workspace root. Each repo is a subdirectory.\n\n")

	// Acceptance criteria
	if len(prog.AcceptanceCriteria) > 0 {
		b.WriteString("## Acceptance Criteria\n\nThe implementation MUST satisfy ALL of the following:\n\n")
		for _, c := range prog.AcceptanceCriteria {
			fmt.Fprintf(&b, "- %s\n", c)
		}
		b.WriteString("\n")
	}

	// CLAUDE.md files
	for _, repo := range prog.TargetRepos {
		claudePath := filepath.Join(workspaceRoot, repo, "CLAUDE.md")
		data, err := os.ReadFile(claudePath)
		if err != nil {
			continue
		}
		content := strings.TrimSpace(string(data))
		fmt.Fprintf(&b, "## %s/CLAUDE.md\n```\n%s\n```\n\n", repo, content)
		pt.AddContext("claude_md:"+repo, content)
	}

	// Context hints
	if len(prog.ContextHints) > 0 {
		b.WriteString("## Pre-Read Context\n\nThe following files were identified as relevant to this task:\n\n")
		for _, hint := range prog.ContextHints {
			content := readContextHint(workspaceRoot, hint)
			b.WriteString(content)
			pt.AddContext("context_hint:"+hint, content)
		}
	}

	// Agent descriptions (so planner knows what agents exist)
	if len(prog.Agents) > 0 {
		b.WriteString("## Agents\n\nThe following sub-agents will execute the plan:\n\n")
		for _, a := range prog.Agents {
			repos := prog.EffectiveTargetRepos(&a)
			fmt.Fprintf(&b, "- **%s** (repos: %s): %s\n", a.Name, strings.Join(repos, ", "), firstLine(a.Instructions))
		}
		b.WriteString("\nDesign your plan with these agents in mind — assign work to specific agents where appropriate.\n\n")
	}

	// Plan output format
	b.WriteString(`## Instructions

1. **Explore the codebase.** Use Read, Glob, Grep, and Bash (read-only commands like ` + "`ls`, `find`, `wc`" + `) to understand the relevant code, patterns, and conventions.

2. **Identify reusable code.** Find existing functions, utilities, components, and patterns that the implementation should reuse.

3. **Produce a plan** with exactly these sections:

### Implementation Plan
For each file to create or modify:
- File path
- What changes to make
- Existing functions/patterns to reuse (with file paths)
- Dependencies on other changes

### Verification
- Commands to run to verify the changes (lint, test, build)
- Expected outcomes

### Questions
- List anything unclear or ambiguous
- If nothing is unclear, write "No questions."

Keep the plan concise and actionable. Focus on what to do, not why.
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

// firstLine returns the first non-empty line of a string.
func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if first, _, found := strings.Cut(s, "\n"); found {
		return first
	}
	return s
}
