package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// BuildReadme constructs a README update prompt for a merged PR.
func BuildReadme(prRepo string, prNumber string, workspaceRoot string, promptsDir string) (string, error) {
	tmpl, err := Template("readme-prompt.md")
	if err != nil {
		return "", fmt.Errorf("loading readme prompt template: %w", err)
	}

	// Fetch PR details via gh CLI
	prTitle := GHPRField(prRepo, prNumber, "title")
	prDescription := GHPRField(prRepo, prNumber, "body")
	prDiff := ghPRDiff(prRepo, prNumber)

	// Extract short repo name (e.g., "app" from "partio-io/app")
	repoShort := prRepo
	if idx := strings.LastIndex(prRepo, "/"); idx >= 0 {
		repoShort = prRepo[idx+1:]
	}

	// Read per-repo custom prompt from prompts/readme-<repo>.md
	var customPrompt string
	customPath := filepath.Join(promptsDir, "readme-"+repoShort+".md")
	if data, err := os.ReadFile(customPath); err == nil {
		customPrompt = strings.TrimSpace(string(data))
	}

	// Read current README.md from target repo
	var currentReadme string
	readmePath := filepath.Join(workspaceRoot, repoShort, "README.md")
	if data, err := os.ReadFile(readmePath); err == nil {
		currentReadme = strings.TrimSpace(string(data))
	}

	// Read repo CLAUDE.md for context
	var repoClaudeMD string
	claudePath := filepath.Join(workspaceRoot, repoShort, "CLAUDE.md")
	if data, err := os.ReadFile(claudePath); err == nil {
		repoClaudeMD = strings.TrimSpace(string(data))
	}

	prRef := prRepo + "#" + prNumber

	result := tmpl
	result = strings.ReplaceAll(result, "{{PR_REF}}", prRef)
	result = strings.ReplaceAll(result, "{{PR_REPO}}", prRepo)
	result = strings.ReplaceAll(result, "{{PR_NUMBER}}", prNumber)
	result = strings.ReplaceAll(result, "{{PR_TITLE}}", prTitle)
	result = strings.ReplaceAll(result, "{{PR_DESCRIPTION}}", prDescription)
	result = strings.ReplaceAll(result, "{{PR_DIFF}}", prDiff)
	result = strings.ReplaceAll(result, "{{CUSTOM_PROMPT}}", customPrompt)
	result = strings.ReplaceAll(result, "{{CURRENT_README}}", currentReadme)
	result = strings.ReplaceAll(result, "{{REPO_CLAUDE_MD}}", repoClaudeMD)

	return result, nil
}
