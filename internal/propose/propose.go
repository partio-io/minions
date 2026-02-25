package propose

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os/exec"
	"regexp"
	"strings"

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
func ExtractFeatures(sourceType, sourceURL, content string) ([]ingest.Feature, error) {
	tmpl, err := prompt.Template("ingest-prompt.md")
	if err != nil {
		return nil, fmt.Errorf("loading ingest template: %w", err)
	}

	fullPrompt := tmpl
	fullPrompt = strings.ReplaceAll(fullPrompt, "{{SOURCE_TYPE}}", sourceType)
	fullPrompt = strings.ReplaceAll(fullPrompt, "{{SOURCE_URL}}", sourceURL)
	fullPrompt = strings.ReplaceAll(fullPrompt, "{{CONTENT}}", content)

	slog.Info("sending content to Claude for feature extraction", "source_type", sourceType, "content_len", len(content))

	cmd := exec.Command("claude", "-p", "--output-format", "json")
	cmd.Stdin = strings.NewReader(fullPrompt)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("claude failed to extract features: %w", err)
	}

	var features []ingest.Feature
	if err := json.Unmarshal(out, &features); err != nil {
		return nil, fmt.Errorf("parsing Claude output as JSON: %w\nOutput: %s", err, string(out))
	}

	return features, nil
}

// ProcessSource fetches a changelog, detects new versions, extracts features, and creates proposal issues.
// Returns the latest version string processed (for updating sources.yaml).
func ProcessSource(src Source, repo string, dryRun bool) (string, error) {
	slog.Info("processing source", "name", src.Name, "url", src.URL, "last_version", src.LastVersion)

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
	features, err := ExtractFeatures(src.Type, sourceRef, combined.String())
	if err != nil {
		return "", fmt.Errorf("extracting features for %s: %w", src.Name, err)
	}

	if len(features) == 0 {
		slog.Info("no relevant features found", "source", src.Name)
		return latestVersion, nil
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
			fmt.Printf("[dry-run] Would create issue: [minion-proposal] %s\n", f.Title)
			fmt.Printf("  Feature ID: %s\n", f.ID)
			fmt.Printf("  Target repos: %s\n", strings.Join(f.TargetRepos, ", "))
			fmt.Println()
			continue
		}

		issueURL, err := CreateProposalIssue(repo, f, src.Name)
		if err != nil {
			slog.Error("creating proposal issue", "feature", f.ID, "error", err)
			continue
		}
		slog.Info("created proposal issue", "feature", f.ID, "url", issueURL)
	}

	return latestVersion, nil
}
