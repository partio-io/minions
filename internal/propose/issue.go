package propose

import (
	"fmt"
	"os/exec"
	"regexp"
	"strings"

	"github.com/partio-io/minions/internal/ingest"
)

// crossRepoRefRe matches shorthand cross-repo references like "org/repo#123".
var crossRepoRefRe = regexp.MustCompile(`([a-zA-Z0-9_.-]+/[a-zA-Z0-9_.-]+)#(\d+)`)

// githubRepoFromURLRe extracts "org/repo" from a GitHub URL.
var githubRepoFromURLRe = regexp.MustCompile(`github\.com/([^/]+/[^/]+)`)

// redirectRef rewrites org/repo#123 shorthand references as markdown links using
// redirect.github.com, which preserves clickability but avoids creating backlinks
// on the referenced issue/PR. Only refs matching sourceRepo are rewritten;
// internal refs (e.g. to partio-io repos) are left as-is. See:
// https://docs.github.com/en/get-started/writing-on-github/working-with-advanced-formatting/autolinked-references-and-urls#avoiding-backlinks-to-linked-references
func redirectRef(s, sourceRepo string) string {
	if sourceRepo == "" {
		return s
	}
	return crossRepoRefRe.ReplaceAllStringFunc(s, func(match string) string {
		m := crossRepoRefRe.FindStringSubmatch(match)
		if m[1] != sourceRepo {
			return match
		}
		return fmt.Sprintf("[%s#%s](https://redirect.github.com/%s/issues/%s)", m[1], m[2], m[1], m[2])
	})
}

// sourceRepoFromURL extracts "org/repo" from a GitHub URL.
func sourceRepoFromURL(url string) string {
	m := githubRepoFromURLRe.FindStringSubmatch(url)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

// IssueExists checks if a proposal issue for the given featureID already exists.
// It searches for issues with the minion-proposal label whose body contains the feature ID.
func IssueExists(repo, featureID string) (bool, error) {
	cmd := exec.Command("gh", "issue", "list",
		"--repo", repo,
		"--label", "minion-proposal",
		"--search", featureID,
		"--json", "number",
		"--limit", "1",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("searching issues: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Empty JSON array means no matches
	return strings.TrimSpace(string(out)) != "[]", nil
}

// CreateProposalIssue creates a GitHub issue with the minion-proposal label.
// If the feature has Plan: true, the minion-planning label is also added to trigger plan generation.
func CreateProposalIssue(repo string, f ingest.Feature, sourceName, sourceType, sourceRepo string) (string, error) {
	title := f.Title
	body := FormatIssueBody(f, sourceName, sourceType, sourceRepo)

	labels := "minion-proposal"
	if f.Plan {
		labels = "minion-proposal,minion-planning"
	}

	cmd := exec.Command("gh", "issue", "create",
		"--repo", repo,
		"--title", title,
		"--label", labels,
		"--body", body,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("creating issue: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return strings.TrimSpace(string(out)), nil
}

// FormatIssueBody generates the full Markdown body for a proposal issue.
// sourceRepo is the org/repo of the monitored source (e.g. "entireio/cli");
// cross-repo refs matching it use redirect.github.com to avoid backlinks.
func FormatIssueBody(f ingest.Feature, sourceName, sourceType, sourceRepo string) string {
	var b strings.Builder

	b.WriteString("## Description\n\n")
	b.WriteString(f.Description)
	b.WriteString("\n\n")

	if f.Why != "" {
		b.WriteString("## Why\n\n")
		b.WriteString(f.Why)
		b.WriteString("\n\n")
	}

	if f.UserRelevance != "" {
		b.WriteString("## User Relevance\n\n")
		b.WriteString(f.UserRelevance)
		b.WriteString("\n\n")
	}

	b.WriteString("## Source\n\n")
	b.WriteString(fmt.Sprintf("- **Origin:** %s\n", redirectRef(f.Source, sourceRepo)))
	b.WriteString(fmt.Sprintf("- **Detected from:** `%s`\n", sourceName))
	b.WriteString("\n")

	b.WriteString("## Target Repos\n\n")
	for _, r := range f.TargetRepos {
		b.WriteString(fmt.Sprintf("- `%s`\n", r))
	}
	b.WriteString("\n")

	b.WriteString("## Acceptance Criteria\n\n")
	for _, ac := range f.AcceptanceCriteria {
		b.WriteString(fmt.Sprintf("- [ ] %s\n", ac))
	}
	b.WriteString("\n")

	if len(f.ContextHints) > 0 {
		b.WriteString("## Context Hints\n\n")
		for _, ch := range f.ContextHints {
			b.WriteString(fmt.Sprintf("- `%s`\n", ch))
		}
		b.WriteString("\n")
	}

	// Embedded program for machine parsing
	b.WriteString("---\n\n")
	b.WriteString("Comment `/minion build` or add the `minion-approved` label to begin implementation.\n\n")
	b.WriteString(embedProgram(f))

	return b.String()
}

// embedProgram generates a hidden HTML comment containing the program as .md format.
func embedProgram(f ingest.Feature) string {
	var p strings.Builder

	// Frontmatter
	p.WriteString("---\n")
	fmt.Fprintf(&p, "id: %s\n", f.ID)
	if f.Source != "" {
		fmt.Fprintf(&p, "source: %s\n", f.Source)
	}
	if len(f.TargetRepos) > 0 {
		p.WriteString("target_repos:\n")
		for _, r := range f.TargetRepos {
			fmt.Fprintf(&p, "  - %s\n", r)
		}
	}
	if len(f.AcceptanceCriteria) > 0 {
		p.WriteString("acceptance_criteria:\n")
		for _, c := range f.AcceptanceCriteria {
			fmt.Fprintf(&p, "  - %s\n", c)
		}
	}
	p.WriteString("pr_labels:\n  - minion\n  - feature\n")
	p.WriteString("---\n\n")

	// Title
	fmt.Fprintf(&p, "# %s\n\n", f.Title)

	// Description
	p.WriteString(f.Description)
	p.WriteString("\n")

	// Context hints as ## Context section
	if len(f.ContextHints) > 0 {
		p.WriteString("\n## Context\n\n")
		for _, ch := range f.ContextHints {
			fmt.Fprintf(&p, "- `%s`\n", ch)
		}
	}

	return fmt.Sprintf("<!-- minion-program\n%s\nminion-program -->", p.String())
}

// ExtractProgramFromIssue extracts the embedded program from an issue body.
// Checks for <!-- minion-program --> first, falls back to <!-- minion-task --> (legacy).
func ExtractProgramFromIssue(body string) string {
	// Try new program format first
	if prog := extractBetweenMarkers(body, "<!-- minion-program\n", "minion-program -->"); prog != "" {
		return prog
	}

	// Fall back to legacy YAML task format — convert to program
	yaml := extractBetweenMarkers(body, "<!-- minion-task\n", "minion-task -->")
	if yaml == "" {
		return ""
	}
	return convertYAMLToProgram(yaml)
}

// extractBetweenMarkers extracts text between start and end markers.
func extractBetweenMarkers(body, startMarker, endMarker string) string {
	startIdx := strings.Index(body, startMarker)
	if startIdx == -1 {
		return ""
	}
	startIdx += len(startMarker)
	endIdx := strings.Index(body[startIdx:], endMarker)
	if endIdx == -1 {
		return ""
	}
	return strings.TrimSpace(body[startIdx : startIdx+endIdx])
}

// convertYAMLToProgram converts legacy embedded YAML task to program .md format.
// This handles the 92 existing issues that have <!-- minion-task --> format.
func convertYAMLToProgram(yamlContent string) string {
	// Parse key fields from YAML using simple line-by-line extraction
	// (avoids importing yaml just for this conversion)
	var id, title, source, description string
	var targetRepos, criteria, contextHints []string
	var inDesc, inTargetRepos, inCriteria, inContextHints bool

	for _, line := range strings.Split(yamlContent, "\n") {
		trimmed := strings.TrimSpace(line)

		// Detect section starts
		if strings.HasPrefix(line, "id:") {
			id = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
			continue
		}
		if strings.HasPrefix(line, "title:") {
			title = strings.TrimSpace(strings.TrimPrefix(line, "title:"))
			title = strings.Trim(title, "'\"")
			continue
		}
		if strings.HasPrefix(line, "source:") {
			source = strings.TrimSpace(strings.TrimPrefix(line, "source:"))
			continue
		}
		if strings.HasPrefix(line, "description:") {
			inDesc = true
			inTargetRepos = false
			inCriteria = false
			inContextHints = false
			// Inline value
			val := strings.TrimSpace(strings.TrimPrefix(line, "description:"))
			if val != "" && val != "|" {
				description = strings.Trim(val, "'\"")
			}
			continue
		}
		if strings.HasPrefix(line, "target_repos:") {
			inDesc = false
			inTargetRepos = true
			inCriteria = false
			inContextHints = false
			continue
		}
		if strings.HasPrefix(line, "acceptance_criteria:") {
			inDesc = false
			inTargetRepos = false
			inCriteria = true
			inContextHints = false
			continue
		}
		if strings.HasPrefix(line, "context_hints:") {
			inDesc = false
			inTargetRepos = false
			inCriteria = false
			inContextHints = true
			continue
		}
		// Other top-level keys end current section
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") && strings.Contains(line, ":") && trimmed != "" {
			inDesc = false
			inTargetRepos = false
			inCriteria = false
			inContextHints = false
			continue
		}

		// Collect values
		if inDesc && (strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "\t")) {
			description += strings.TrimPrefix(strings.TrimPrefix(line, "    "), "  ") + "\n"
		}
		if inTargetRepos && strings.HasPrefix(trimmed, "- ") {
			targetRepos = append(targetRepos, strings.TrimSpace(trimmed[2:]))
		}
		if inCriteria && strings.HasPrefix(trimmed, "- ") {
			criteria = append(criteria, strings.TrimSpace(trimmed[2:]))
		}
		if inContextHints && strings.HasPrefix(trimmed, "- ") {
			contextHints = append(contextHints, strings.TrimSpace(trimmed[2:]))
		}
	}

	// Build program .md
	var p strings.Builder
	p.WriteString("---\n")
	if id != "" {
		fmt.Fprintf(&p, "id: %s\n", id)
	}
	if source != "" {
		fmt.Fprintf(&p, "source: %s\n", source)
	}
	if len(targetRepos) > 0 {
		p.WriteString("target_repos:\n")
		for _, r := range targetRepos {
			fmt.Fprintf(&p, "  - %s\n", r)
		}
	}
	if len(criteria) > 0 {
		p.WriteString("acceptance_criteria:\n")
		for _, c := range criteria {
			fmt.Fprintf(&p, "  - %s\n", c)
		}
	}
	p.WriteString("pr_labels:\n  - minion\n  - feature\n")
	p.WriteString("---\n\n")

	if title != "" {
		fmt.Fprintf(&p, "# %s\n\n", title)
	}
	if description != "" {
		p.WriteString(strings.TrimSpace(description))
		p.WriteString("\n")
	}
	if len(contextHints) > 0 {
		p.WriteString("\n## Context\n\n")
		for _, ch := range contextHints {
			fmt.Fprintf(&p, "- `%s`\n", ch)
		}
	}

	return p.String()
}
