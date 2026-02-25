package ingest

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
)

var (
	reScript = regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`)
	reStyle  = regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)
	reTags   = regexp.MustCompile(`<[^>]+>`)
	reSpaces = regexp.MustCompile(`\s+`)
)

// FetchBlog fetches a blog post and strips HTML to plain text.
func FetchBlog(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", fmt.Errorf("fetching blog: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("fetching blog: HTTP %d", resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("reading blog: %w", err)
	}

	// Strip HTML in pure Go (replaces python3 dependency)
	text := string(data)
	text = reScript.ReplaceAllString(text, "")
	text = reStyle.ReplaceAllString(text, "")
	text = reTags.ReplaceAllString(text, " ")
	text = reSpaces.ReplaceAllString(text, " ")
	text = strings.TrimSpace(text)

	return text, nil
}
