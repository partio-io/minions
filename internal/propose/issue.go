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
// on the referenced issue/PR.
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
	return strings.TrimSpace(string(out)) != "[]", nil
}

// CreateProposalIssue creates a GitHub issue with the minion-proposal label.
// programPath is the relative path to the program file in the repo.
func CreateProposalIssue(repo string, f ingest.Feature, sourceName, sourceRepo, programPath string) (string, error) {
	body := FormatIssueBody(f, sourceName, sourceRepo, programPath)

	labels := "minion-proposal"
	if f.Plan {
		labels = "minion-proposal,minion-planning"
	}

	cmd := exec.Command("gh", "issue", "create",
		"--repo", repo,
		"--title", f.Title,
		"--label", labels,
		"--body", body,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("creating issue: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// FormatIssueBody generates the Markdown body for a proposal issue.
// The issue references the program file rather than embedding it.
func FormatIssueBody(f ingest.Feature, sourceName, sourceRepo, programPath string) string {
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
	fmt.Fprintf(&b, "- **Origin:** %s\n", redirectRef(f.Source, sourceRepo))
	fmt.Fprintf(&b, "- **Detected from:** `%s`\n", sourceName)
	b.WriteString("\n")

	b.WriteString("## Target Repos\n\n")
	for _, r := range f.TargetRepos {
		fmt.Fprintf(&b, "- `%s`\n", r)
	}
	b.WriteString("\n")

	b.WriteString("## Acceptance Criteria\n\n")
	for _, ac := range f.AcceptanceCriteria {
		fmt.Fprintf(&b, "- [ ] %s\n", ac)
	}
	b.WriteString("\n")

	if len(f.ContextHints) > 0 {
		b.WriteString("## Context Hints\n\n")
		for _, ch := range f.ContextHints {
			fmt.Fprintf(&b, "- `%s`\n", ch)
		}
		b.WriteString("\n")
	}

	// Program reference and action
	b.WriteString("---\n\n")
	fmt.Fprintf(&b, "**Program:** [`%s`](%s)\n\n", programPath, programPath)
	b.WriteString("Comment `/minion build` or add the `minion-approved` label to begin implementation.\n\n")
	fmt.Fprintf(&b, "<!-- program: %s -->", programPath)

	return b.String()
}

// BuildProgramFile generates the .md program content for a feature.
func BuildProgramFile(f ingest.Feature) string {
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
	p.WriteString(strings.TrimSpace(f.Description))
	p.WriteString("\n")

	// Context hints
	if len(f.ContextHints) > 0 {
		p.WriteString("\n## Context\n\n")
		for _, ch := range f.ContextHints {
			fmt.Fprintf(&p, "- `%s`\n", ch)
		}
	}

	return p.String()
}

// ExtractProgramPathFromIssue extracts the program file path from an issue body.
// Looks for <!-- program: .minions/programs/foo.md --> marker.
func ExtractProgramPathFromIssue(body string) string {
	const startMarker = "<!-- program: "
	const endMarker = " -->"

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

// commitAndPushPrograms stages, commits, and pushes new program files.
func commitAndPushPrograms(repoPath string, paths []string) error {
	for _, p := range paths {
		cmd := exec.Command("git", "-C", repoPath, "add", p)
		if out, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("staging %s: %s: %w", p, string(out), err)
		}
	}

	cmd := exec.Command("git", "-C", repoPath, "commit", "-m", "chore: add minion program proposals")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("committing: %s: %w", string(out), err)
	}

	cmd = exec.Command("git", "-C", repoPath, "push")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("pushing: %s: %w", string(out), err)
	}

	return nil
}
