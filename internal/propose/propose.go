package propose

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"

	claudesdk "github.com/partio-io/claude-agent-sdk-go"

	"github.com/partio-io/minions/internal/claude"
	"github.com/partio-io/minions/internal/ingest"
	"github.com/partio-io/minions/internal/prompt"
)

// versionHeaderRe matches ## or # headers containing version-like strings (e.g., "## 0.4.5", "## [1.2.3]").
var versionHeaderRe = regexp.MustCompile(`^#{1,2}\s+\[?(\d+\.\d+[\.\d]*)`)

// DetectNewVersions scans changelog content for version headers newer than lastVersion.
// Returns a list of version strings found after lastVersion (in document order).
// If lastVersion is empty, returns all detected versions.
func DetectNewVersions(content, lastVersion string) []string {
	lines := strings.Split(content, "\n")
	var versions []string

	for _, line := range lines {
		m := versionHeaderRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		ver := m[1]
		if lastVersion != "" && ver == lastVersion {
			// Stop at the last-processed version
			break
		}
		versions = append(versions, ver)
	}

	return versions
}

// ExtractFeatures sends changelog content to Claude and returns extracted features.
// This duplicates the Claude-calling portion of ingest.GenerateTasks but skips file writing.
func ExtractFeatures(ctx context.Context, sourceType, sourceURL, content string) ([]ingest.Feature, error) {
	tmpl, err := prompt.Template("ingest-prompt.md")
	if err != nil {
		return nil, fmt.Errorf("loading ingest template: %w", err)
	}

	fullPrompt := tmpl
	fullPrompt = strings.ReplaceAll(fullPrompt, "{{SOURCE_TYPE}}", sourceType)
	fullPrompt = strings.ReplaceAll(fullPrompt, "{{SOURCE_URL}}", sourceURL)
	fullPrompt = strings.ReplaceAll(fullPrompt, "{{CONTENT}}", content)

	slog.Info("sending content to Claude for feature extraction", "source_type", sourceType, "content_len", len(content))

	resultMsg, err := claudesdk.Prompt(ctx, fullPrompt)
	if err != nil {
		return nil, fmt.Errorf("claude failed to extract features: %w", err)
	}
	if resultMsg.IsError || resultMsg.Result == nil {
		return nil, fmt.Errorf("claude returned error: subtype=%s", resultMsg.Subtype)
	}

	resultStr := claude.StripCodeFences(*resultMsg.Result)

	var features []ingest.Feature
	if err := json.Unmarshal([]byte(resultStr), &features); err != nil {
		return nil, fmt.Errorf("parsing features JSON: %w\nOutput: %s", err, resultStr)
	}

	return features, nil
}

// ProcessSource dispatches to the appropriate handler based on source type.
// Returns the latest version/cursor string processed (for updating sources.yaml).
func ProcessSource(ctx context.Context, src Source, repo string, dryRun bool) (string, error) {
	slog.Info("processing source", "name", src.Name, "type", src.Type, "url", src.URL, "last_version", src.LastVersion)

	switch src.Type {
	case "issues", "pulls":
		return processGitHubItems(ctx, src, repo, dryRun)
	default:
		return processChangelog(ctx, src, repo, dryRun)
	}
}

// processChangelog handles changelog-type sources (original behavior).
func processChangelog(ctx context.Context, src Source, repo string, dryRun bool) (string, error) {
	content, err := ingest.FetchChangelog(src.URL)
	if err != nil {
		return "", fmt.Errorf("fetching changelog for %s: %w", src.Name, err)
	}
	if content == "" {
		return "", fmt.Errorf("empty changelog from %s", src.URL)
	}

	newVersions := DetectNewVersions(content, src.LastVersion)
	if len(newVersions) == 0 {
		slog.Info("no new versions detected", "source", src.Name)
		return src.LastVersion, nil
	}

	slog.Info("detected new versions", "source", src.Name, "versions", newVersions)

	latestVersion := newVersions[0]

	// Combine content from all new versions for feature extraction
	var combined strings.Builder
	for _, ver := range newVersions {
		section := ingest.ExtractVersion(content, ver)
		if section != "" {
			combined.WriteString(section)
			combined.WriteString("\n\n")
		}
	}

	if combined.Len() == 0 {
		slog.Warn("no content extracted for new versions", "source", src.Name)
		return latestVersion, nil
	}

	sourceRef := fmt.Sprintf("%s (%s)", src.URL, strings.Join(newVersions, ", "))
	features, err := ExtractFeatures(ctx, src.Type, sourceRef, combined.String())
	if err != nil {
		return "", fmt.Errorf("extracting features for %s: %w", src.Name, err)
	}

	createProposalIssues(features, repo, src, dryRun)
	return latestVersion, nil
}

// processGitHubItems handles issues and pulls source types.
func processGitHubItems(ctx context.Context, src Source, repo string, dryRun bool) (string, error) {
	sourceRepo := src.Repo
	if sourceRepo == "" {
		return "", fmt.Errorf("source %s has type %q but no repo field", src.Name, src.Type)
	}

	var items []ingest.GHItem
	var err error
	switch src.Type {
	case "issues":
		items, err = ingest.FetchRepoIssues(sourceRepo)
	case "pulls":
		items, err = ingest.FetchRepoPulls(sourceRepo)
	}
	if err != nil {
		return "", fmt.Errorf("fetching %s for %s: %w", src.Type, src.Name, err)
	}

	// Parse last_version as the last-seen item number
	lastSeen := 0
	if src.LastVersion != "" && src.LastVersion != "0" {
		lastSeen, err = strconv.Atoi(src.LastVersion)
		if err != nil {
			return "", fmt.Errorf("parsing last_version %q as int: %w", src.LastVersion, err)
		}
	}

	// Filter to items newer than lastSeen
	var newItems []ingest.GHItem
	for _, item := range items {
		if item.Number > lastSeen {
			newItems = append(newItems, item)
		}
	}

	if len(newItems) == 0 {
		slog.Info("no new items", "source", src.Name, "type", src.Type)
		return src.LastVersion, nil
	}

	slog.Info("found new items", "source", src.Name, "type", src.Type, "count", len(newItems))

	// Format items as markdown for feature extraction
	var content strings.Builder
	highestNumber := 0
	for _, item := range newItems {
		fmt.Fprintf(&content, "## #%d: %s\n\n%s\n\n", item.Number, item.Title, item.Body)
		if item.Number > highestNumber {
			highestNumber = item.Number
		}
	}

	sourceRef := fmt.Sprintf("%s/%s (%s)", sourceRepo, src.Type, src.Name)
	features, err := ExtractFeatures(ctx, src.Type, sourceRef, content.String())
	if err != nil {
		return "", fmt.Errorf("extracting features for %s: %w", src.Name, err)
	}

	createProposalIssues(features, repo, src, dryRun)
	return strconv.Itoa(highestNumber), nil
}

// createProposalIssues creates proposal issues for extracted features.
// It derives the source repo from the Source config to scope backlink suppression.
func createProposalIssues(features []ingest.Feature, repo string, src Source, dryRun bool) {
	sourceRepo := src.Repo
	if sourceRepo == "" {
		sourceRepo = sourceRepoFromURL(src.URL)
	}

	if len(features) == 0 {
		slog.Info("no relevant features found", "source", src.Name)
		return
	}

	slog.Info("extracted features", "source", src.Name, "count", len(features))

	for _, f := range features {
		exists, err := IssueExists(repo, f.ID)
		if err != nil {
			slog.Error("checking issue existence", "feature", f.ID, "error", err)
			continue
		}
		if exists {
			slog.Info("issue already exists, skipping", "feature", f.ID)
			continue
		}

		if dryRun {
			fmt.Printf("[dry-run] Would create issue: %s\n", f.Title)
			fmt.Printf("  Feature ID: %s\n", f.ID)
			fmt.Printf("  Target repos: %s\n", strings.Join(f.TargetRepos, ", "))
			fmt.Println()
			continue
		}

		issueURL, err := CreateProposalIssue(repo, f, src.Name, src.Type, sourceRepo)
		if err != nil {
			slog.Error("creating proposal issue", "feature", f.ID, "error", err)
			continue
		}
		slog.Info("created proposal issue", "feature", f.ID, "url", issueURL)
	}
}
