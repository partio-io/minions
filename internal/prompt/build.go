package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/partio-io/minions/internal/task"
)

// repoBuildInfo maps repo names to their build command descriptions.
var repoBuildInfo = map[string]string{
	"cli":       "Go CLI (build: `make lint && make test`)",
	"app":       "Next.js dashboard (build: `npm run lint && npm run build`)",
	"docs":      "Mintlify docs (verify MDX frontmatter + mint.json)",
	"site":      "Next.js marketing site (build: `npm run lint && npm run build`)",
	"extension": "Browser extension",
}

// BuildTask constructs a full minion prompt from a task spec, CLAUDE.md files, and context hints.
func BuildTask(t *task.Task, workspaceRoot string) (string, error) {
	tmpl, err := Template("prompt.md")
	if err != nil {
		return "", fmt.Errorf("loading prompt template: %w", err)
	}

	// Build target repos text
	var reposBuilder strings.Builder
	for _, repo := range t.TargetRepos {
		info, ok := repoBuildInfo[repo]
		if !ok {
			info = "Unknown repo type"
		}
		fmt.Fprintf(&reposBuilder, "- `%s/` — %s\n", repo, info)
	}

	// Build acceptance criteria text
	var criteriaBuilder strings.Builder
	for _, c := range t.AcceptanceCriteria {
		fmt.Fprintf(&criteriaBuilder, "- %s\n", c)
	}

	// Build CLAUDE.md contents
	var claudeMDBuilder strings.Builder
	for _, repo := range t.TargetRepos {
		claudePath := filepath.Join(workspaceRoot, repo, "CLAUDE.md")
		data, err := os.ReadFile(claudePath)
		if err != nil {
			continue
		}
		fmt.Fprintf(&claudeMDBuilder, "### %s/CLAUDE.md\n```\n%s\n```\n\n", repo, strings.TrimSpace(string(data)))
	}

	// Build context hints contents
	var hintsBuilder strings.Builder
	for _, hint := range t.ContextHints {
		hintPath := filepath.Join(workspaceRoot, hint)
		info, err := os.Stat(hintPath)
		if err != nil {
			fmt.Fprintf(&hintsBuilder, "### %s\n*(file not found)*\n\n", hint)
			continue
		}
		if info.IsDir() {
			entries, err := os.ReadDir(hintPath)
			if err != nil {
				fmt.Fprintf(&hintsBuilder, "### %s (directory listing)\n```\n(error reading directory)\n```\n\n", hint)
				continue
			}
			var listing strings.Builder
			for _, e := range entries {
				fmt.Fprintln(&listing, e.Name())
			}
			fmt.Fprintf(&hintsBuilder, "### %s (directory listing)\n```\n%s```\n\n", hint, listing.String())
		} else {
			data, err := os.ReadFile(hintPath)
			if err != nil {
				fmt.Fprintf(&hintsBuilder, "### %s\n*(error reading file)*\n\n", hint)
				continue
			}
			fmt.Fprintf(&hintsBuilder, "### %s\n```\n%s\n```\n\n", hint, strings.TrimSpace(string(data)))
		}
	}

	// Perform substitutions
	result := tmpl
	result = strings.ReplaceAll(result, "{{TITLE}}", t.Title)
	result = strings.ReplaceAll(result, "{{DESCRIPTION}}", t.Description)
	result = strings.ReplaceAll(result, "{{TARGET_REPOS}}", reposBuilder.String())
	result = strings.ReplaceAll(result, "{{ACCEPTANCE_CRITERIA}}", criteriaBuilder.String())
	result = strings.ReplaceAll(result, "{{CLAUDE_MD_CONTENTS}}", claudeMDBuilder.String())
	result = strings.ReplaceAll(result, "{{CONTEXT_HINTS_CONTENTS}}", hintsBuilder.String())

	return result, nil
}
