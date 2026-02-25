package prompt

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// BuildDoc constructs a documentation update prompt for a PR.
func BuildDoc(prRepo string, prNumber string, workspaceRoot string) (string, error) {
	tmpl, err := Template("doc-prompt.md")
	if err != nil {
		return "", fmt.Errorf("loading doc prompt template: %w", err)
	}

	// Fetch PR details via gh CLI
	prTitle := GHPRField(prRepo, prNumber, "title")
	prDescription := GHPRField(prRepo, prNumber, "body")
	prDiff := ghPRDiff(prRepo, prNumber)

	// Read docs CLAUDE.md
	var docsClaudeMD string
	claudePath := filepath.Join(workspaceRoot, "docs", "CLAUDE.md")
	if data, err := os.ReadFile(claudePath); err == nil {
		docsClaudeMD = strings.TrimSpace(string(data))
	}

	prRef := prRepo + "#" + prNumber

	result := tmpl
	result = strings.ReplaceAll(result, "{{PR_REF}}", prRef)
	result = strings.ReplaceAll(result, "{{PR_REPO}}", prRepo)
	result = strings.ReplaceAll(result, "{{PR_NUMBER}}", prNumber)
	result = strings.ReplaceAll(result, "{{PR_TITLE}}", prTitle)
	result = strings.ReplaceAll(result, "{{PR_DESCRIPTION}}", prDescription)
	result = strings.ReplaceAll(result, "{{PR_DIFF}}", prDiff)
	result = strings.ReplaceAll(result, "{{DOCS_CLAUDE_MD}}", docsClaudeMD)

	return result, nil
}

// GHPRField fetches a single field from a GitHub PR via the gh CLI.
func GHPRField(repo, number, field string) string {
	cmd := exec.Command("gh", "pr", "view", number, "--repo", repo, "--json", field, "-q", "."+field)
	out, err := cmd.Output()
	if err != nil {
		return "Unknown"
	}
	return strings.TrimSpace(string(out))
}

func ghPRDiff(repo, number string) string {
	cmd := exec.Command("gh", "pr", "diff", number, "--repo", repo)
	out, err := cmd.Output()
	if err != nil {
		return "Unable to fetch diff"
	}
	return string(out)
}
