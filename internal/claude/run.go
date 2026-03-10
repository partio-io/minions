package claude

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// Opts configures a headless Claude Code invocation.
type Opts struct {
	Prompt       string
	CWD          string
	MaxTurns     int
	AllowedTools string // Comma-separated tool list. Defaults to "Edit,Write,Read,Glob,Grep,Bash".
	LogFile      string // If set, tee stdout to this file for debug observability
}

// Run executes `claude -p` in headless mode with the given options.
func Run(opts Opts) error {
	if opts.MaxTurns == 0 {
		opts.MaxTurns = 30
	}

	allowedTools := opts.AllowedTools
	if allowedTools == "" {
		allowedTools = "Edit,Write,Read,Glob,Grep,Bash"
	}

	args := []string{
		"-p", opts.Prompt,
		"--allowedTools", allowedTools,
		"--max-turns", strconv.Itoa(opts.MaxTurns),
		"--output-format", "stream-json",
		"--verbose",
	}

	slog.Info("running claude", "cwd", opts.CWD, "max_turns", opts.MaxTurns)

	cmd := exec.Command("claude", args...)
	cmd.Dir = opts.CWD
	cmd.Stderr = os.Stderr

	stdout := io.Writer(os.Stdout)
	if opts.LogFile != "" {
		f, err := os.Create(opts.LogFile)
		if err != nil {
			slog.Warn("failed to create claude log file, continuing without it", "path", opts.LogFile, "error", err)
		} else {
			defer f.Close()
			stdout = io.MultiWriter(os.Stdout, f)
		}
	}
	cmd.Stdout = stdout

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude exited with error: %w", err)
	}
	return nil
}

// claudeResult represents the JSON envelope from `claude -p --output-format json`.
type claudeResult struct {
	Result  string `json:"result"`
	IsError bool   `json:"is_error"`
}

// ExtractResult unwraps the JSON envelope from `claude -p --output-format json`
// and returns the inner result string with markdown code fences stripped.
func ExtractResult(raw []byte) (string, error) {
	var cr claudeResult
	if err := json.Unmarshal(raw, &cr); err != nil {
		return "", fmt.Errorf("parsing claude JSON envelope: %w", err)
	}
	if cr.IsError {
		return "", fmt.Errorf("claude returned error: %s", cr.Result)
	}
	return stripCodeFences(cr.Result), nil
}

// stripCodeFences removes ```json ... ``` wrapping if present.
func stripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i != -1 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}
