package ingest

import (
	"encoding/json"
	"fmt"
	"os/exec"
	"regexp"
	"strings"
)

type ghIssue struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

var nonAlphaNum = regexp.MustCompile(`[^a-z0-9]+`)

// FetchIssues fetches labeled issues from a GitHub repo using `gh issue list`.
func FetchIssues(repo, label string) ([]ghIssue, error) {
	if label == "" {
		label = "minion-ready"
	}

	cmd := exec.Command("gh", "issue", "list",
		"--repo", repo,
		"--label", label,
		"--json", "number,title,body",
		"--limit", "50",
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("fetching issues from %s: %w", repo, err)
	}

	var issues []ghIssue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing issues JSON: %w", err)
	}

	return issues, nil
}

// IssueToTaskID converts an issue title to a kebab-case task ID.
func IssueToTaskID(title string) string {
	id := strings.ToLower(title)
	id = nonAlphaNum.ReplaceAllString(id, "-")
	id = strings.Trim(id, "-")
	if len(id) > 60 {
		id = id[:60]
		id = strings.TrimRight(id, "-")
	}
	return id
}

// RepoShortName extracts the short name from an owner/repo string.
func RepoShortName(repo string) string {
	parts := strings.Split(repo, "/")
	return parts[len(parts)-1]
}
