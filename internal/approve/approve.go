package approve

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"strings"
	"time"
)

// Issue represents a GitHub issue returned by the gh CLI.
type Issue struct {
	Number    int       `json:"number"`
	Title     string    `json:"title"`
	Body      string    `json:"body"`
	CreatedAt time.Time `json:"createdAt"`
	Labels    []Label   `json:"labels"`
}

// Label represents a GitHub label.
type Label struct {
	Name string `json:"name"`
}

// HasLabel checks if the issue has a specific label.
func (i Issue) HasLabel(name string) bool {
	for _, l := range i.Labels {
		if l.Name == name {
			return true
		}
	}
	return false
}

// ListProposals returns open issues with the minion-proposal label.
func ListProposals(repo string) ([]Issue, error) {
	out, err := exec.Command("gh", "issue", "list",
		"--repo", repo,
		"--label", "minion-proposal",
		"--state", "open",
		"--json", "number,title,createdAt,labels,body",
		"--limit", "100",
	).Output()
	if err != nil {
		return nil, fmt.Errorf("listing proposals: %w", err)
	}

	var issues []Issue
	if err := json.Unmarshal(out, &issues); err != nil {
		return nil, fmt.Errorf("parsing issues: %w", err)
	}

	return issues, nil
}

// blockingLabels are labels that prevent auto-approval.
var blockingLabels = []string{
	"do-not-build",
	"minion-approved",
	"minion-executing",
	"minion-done",
	"minion-failed",
}

// ShouldApprove checks whether an issue is eligible for auto-approval.
// Returns (true, "") if eligible, or (false, reason) if not.
func ShouldApprove(issue Issue, delay time.Duration) (bool, string) {
	for _, label := range blockingLabels {
		if issue.HasLabel(label) {
			return false, fmt.Sprintf("has label %q", label)
		}
	}

	age := time.Since(issue.CreatedAt)
	if age < delay {
		remaining := delay - age
		return false, fmt.Sprintf("too young (%.0fh remaining)", remaining.Hours())
	}

	// Check for embedded task YAML
	if !hasTaskYAML(issue.Body) {
		return false, "no embedded minion-task YAML"
	}

	return true, ""
}

// hasTaskYAML checks if the issue body contains the minion-task marker.
func hasTaskYAML(body string) bool {
	return strings.Contains(body, "<!-- minion-task")
}

// Approve adds the minion-approved label to an issue and posts a comment.
func Approve(repo string, issueNumber int) error {
	// Add label
	err := exec.Command("gh", "issue", "edit",
		"--repo", repo,
		fmt.Sprintf("%d", issueNumber),
		"--add-label", "minion-approved",
	).Run()
	if err != nil {
		return fmt.Errorf("adding minion-approved label: %w", err)
	}

	// Post comment
	comment := "Auto-approved after review window. Minion execution will begin shortly."
	err = exec.Command("gh", "issue", "comment",
		"--repo", repo,
		fmt.Sprintf("%d", issueNumber),
		"--body", comment,
	).Run()
	if err != nil {
		slog.Warn("failed to post approval comment", "issue", issueNumber, "error", err)
		// Non-fatal: the label is what matters
	}

	return nil
}
