package ingest

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// GHItem represents a GitHub issue or pull request.
type GHItem struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

// FetchRepoPulls fetches recent pull requests from a GitHub repo using `gh pr list`.
func FetchRepoPulls(repo string) ([]GHItem, error) {
	cmd := exec.Command("gh", "pr", "list",
		"--repo", repo,
		"--state", "all",
		"--json", "number,title,body",
		"--limit", "50",
	)

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("fetching pulls from %s: %w", repo, err)
	}

	var items []GHItem
	if err := json.Unmarshal(out, &items); err != nil {
		return nil, fmt.Errorf("parsing pulls JSON: %w", err)
	}

	return items, nil
}
