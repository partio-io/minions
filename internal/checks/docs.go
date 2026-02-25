package checks

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func runDocs(repoPath string) (string, error) {
	var allOutput string

	// Validate mint.json is valid JSON
	mintPath := filepath.Join(repoPath, "mint.json")
	data, err := os.ReadFile(mintPath)
	if err != nil {
		return "", fmt.Errorf("reading mint.json: %w", err)
	}
	if !json.Valid(data) {
		return "FAIL: mint.json is invalid JSON", fmt.Errorf("mint.json is invalid JSON")
	}
	allOutput += "PASS: mint.json is valid JSON\n"

	// Check MDX files have frontmatter
	var failed []string
	err = filepath.Walk(repoPath, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if info.IsDir() || filepath.Ext(path) != ".mdx" {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if !strings.HasPrefix(string(content), "---") {
			rel, _ := filepath.Rel(repoPath, path)
			failed = append(failed, rel)
		}
		return nil
	})
	if err != nil {
		return allOutput, fmt.Errorf("walking docs directory: %w", err)
	}

	if len(failed) > 0 {
		msg := fmt.Sprintf("FAIL: Missing frontmatter in: %s", strings.Join(failed, ", "))
		allOutput += msg + "\n"
		return allOutput, fmt.Errorf("MDX frontmatter check failed")
	}
	allOutput += "PASS: All MDX files have frontmatter\n"

	// Run mintlify build if available
	if _, err := exec.LookPath("mintlify"); err == nil {
		cmd := exec.Command("mintlify", "build")
		cmd.Dir = repoPath
		out, err := cmd.CombinedOutput()
		allOutput += string(out) + "\n"
		if err != nil {
			return allOutput, fmt.Errorf("mintlify build failed: %w", err)
		}
	} else {
		allOutput += "SKIP: mintlify not installed\n"
	}

	return allOutput, nil
}
