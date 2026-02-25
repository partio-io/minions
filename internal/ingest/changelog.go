package ingest

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// FetchChangelog fetches raw changelog content from a GitHub URL.
func FetchChangelog(url string) (string, error) {
	// Convert GitHub blob URL to raw URL
	rawURL := url
	rawURL = strings.Replace(rawURL, "github.com", "raw.githubusercontent.com", 1)
	rawURL = strings.Replace(rawURL, "/blob/", "/", 1)

	resp, err := http.Get(rawURL)
	if err != nil {
		return "", fmt.Errorf("fetching changelog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching changelog: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading changelog: %w", err)
	}

	return string(data), nil
}

// ExtractVersion extracts a specific version section from changelog content.
func ExtractVersion(content, version string) string {
	lines := strings.Split(content, "\n")
	var result []string
	found := false

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") || strings.HasPrefix(line, "# ") {
			if found {
				break
			}
			if strings.Contains(line, version) {
				found = true
			}
		}
		if found {
			result = append(result, line)
		}
	}

	return strings.Join(result, "\n")
}
