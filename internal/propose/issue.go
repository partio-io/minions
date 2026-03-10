package propose

import (
	"fmt"
	"os/exec"
	"strings"

	"github.com/partio-io/minions/internal/ingest"

	"gopkg.in/yaml.v3"
)

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
func CreateProposalIssue(repo string, f ingest.Feature, sourceName, sourceType string) (string, error) {
	title := f.Title
	body := FormatIssueBody(f, sourceName, sourceType)

	cmd := exec.Command("gh", "issue", "create",
		"--repo", repo,
		"--title", title,
		"--label", "minion-proposal",
		"--body", body,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("creating issue: %s: %w", strings.TrimSpace(string(out)), err)
	}

	return strings.TrimSpace(string(out)), nil
}

// FormatIssueBody generates the full Markdown body for a proposal issue.
func FormatIssueBody(f ingest.Feature, sourceName, sourceType string) string {
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
	b.WriteString(fmt.Sprintf("- **Origin:** %s\n", f.Source))
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

	// Embedded task YAML for machine parsing
	b.WriteString("---\n\n")
	b.WriteString("Comment `/minion build` or add the `minion-approved` label to begin implementation.\n\n")
	b.WriteString(embedTaskYAML(f, sourceType))

	return b.String()
}

// embedTaskYAML generates a hidden HTML comment containing the task spec as YAML.
func embedTaskYAML(f ingest.Feature, sourceType string) string {
	task := struct {
		ID                 string   `yaml:"id"`
		Title              string   `yaml:"title"`
		Source             string   `yaml:"source"`
		SourceType         string   `yaml:"source_type"`
		Description        string   `yaml:"description"`
		Why                string   `yaml:"why,omitempty"`
		UserRelevance      string   `yaml:"user_relevance,omitempty"`
		TargetRepos        []string `yaml:"target_repos"`
		ContextHints       []string `yaml:"context_hints"`
		AcceptanceCriteria []string `yaml:"acceptance_criteria"`
		PRLabels           []string `yaml:"pr_labels"`
	}{
		ID:                 f.ID,
		Title:              f.Title,
		Source:             f.Source,
		SourceType:         sourceType,
		Description:        f.Description,
		Why:                f.Why,
		UserRelevance:      f.UserRelevance,
		TargetRepos:        f.TargetRepos,
		ContextHints:       f.ContextHints,
		AcceptanceCriteria: f.AcceptanceCriteria,
		PRLabels:           []string{"minion", "feature"},
	}

	data, err := yaml.Marshal(&task)
	if err != nil {
		return ""
	}

	return fmt.Sprintf("<!-- minion-task\n%s minion-task -->", string(data))
}

// ExtractTaskYAMLFromIssue extracts the embedded task YAML from an issue body.
// Returns the YAML string between <!-- minion-task and minion-task --> markers.
func ExtractTaskYAMLFromIssue(body string) string {
	const startMarker = "<!-- minion-task\n"
	const endMarker = "minion-task -->"

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
