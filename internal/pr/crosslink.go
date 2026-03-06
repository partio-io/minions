package pr

import (
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var prURLPattern = regexp.MustCompile(`https://github\.com/([^/]+/[^/]+)/pull/(\d+)`)

// Crosslink adds cross-reference comments between two PRs.
func Crosslink(prURL1, prURL2 string) {
	comment(prURL1, fmt.Sprintf("Related PR: %s", prURL2))
	comment(prURL2, fmt.Sprintf("Related PR: %s", prURL1))
}

// CreateAndLinkAll creates PRs in all repos that have changes and cross-links them.
func CreateAndLinkAll(taskID, title, workspaceRoot, labelsCSV string, repos []string) ([]string, error) {
	var labels []string
	if labelsCSV != "" {
		labels = strings.Split(labelsCSV, ",")
	}

	var prURLs []string

	for _, repo := range repos {
		wtPath := filepath.Join(workspaceRoot, repo, ".minion-worktrees", taskID)
		repoFullName := "partio-io/" + repo

		prURL, err := Create(wtPath, repoFullName, taskID, title, labels)
		if err != nil {
			slog.Error("failed to create PR", "repo", repo, "error", err)
			continue
		}
		if prURL != "" {
			prURLs = append(prURLs, prURL)
			slog.Info("created PR", "url", prURL)
		}
	}

	// Cross-link all PRs
	for i := 0; i < len(prURLs); i++ {
		for j := i + 1; j < len(prURLs); j++ {
			Crosslink(prURLs[i], prURLs[j])
		}
	}

	if len(prURLs) == 0 && len(repos) > 0 {
		return nil, fmt.Errorf("no PRs created across %d repos", len(repos))
	}

	return prURLs, nil
}

// CreateDocsPR creates a PR in the docs repo for a documentation update.
func CreateDocsPR(prRepo, prNumber, branchName, sourcePRTitle string) (string, error) {
	title := fmt.Sprintf("[docs] Update for %s#%s: %s", prRepo, prNumber, sourcePRTitle)
	body := fmt.Sprintf(`## Summary

Automated documentation update for %s#%s.

**Source PR:** https://github.com/%s/pull/%s

---

*This PR was created by the doc-minion. Please review carefully.*`, prRepo, prNumber, prRepo, prNumber)

	// Ensure labels exist in target repo (ignore errors for already-existing labels)
	for _, l := range []string{"minion", "documentation"} {
		create := exec.Command("gh", "label", "create", l, "--repo", "partio-io/docs")
		_ = create.Run()
	}

	args := []string{
		"pr", "create",
		"--repo", "partio-io/docs",
		"--head", branchName,
		"--title", title,
		"--body", body,
		"--label", "minion",
		"--label", "documentation",
	}

	cmd := exec.Command("gh", args...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("creating docs PR: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return strings.TrimSpace(string(out)), nil
}

// CommentOnPR adds a comment to a PR by repo and number.
func CommentOnPR(repo, number, body string) {
	cmd := exec.Command("gh", "pr", "comment", number, "--repo", repo, "--body", body)
	if err := cmd.Run(); err != nil {
		slog.Warn("failed to comment on PR", "repo", repo, "number", number, "error", err)
	}
}

func comment(prURL, body string) {
	m := prURLPattern.FindStringSubmatch(prURL)
	if len(m) < 3 {
		return
	}
	repo, num := m[1], m[2]

	cmd := exec.Command("gh", "pr", "comment", num, "--repo", repo, "--body", body)
	if err := cmd.Run(); err != nil {
		slog.Warn("failed to add PR comment", "pr", prURL, "error", err)
	}
}
