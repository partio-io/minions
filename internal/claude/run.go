package claude

import (
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
)

// Opts configures a headless Claude Code invocation.
type Opts struct {
	Prompt   string
	CWD      string
	MaxTurns int
}

// Run executes `claude -p` in headless mode with the given options.
func Run(opts Opts) error {
	if opts.MaxTurns == 0 {
		opts.MaxTurns = 30
	}

	args := []string{
		"-p", opts.Prompt,
		"--allowedTools", "Edit,Write,Read,Glob,Grep,Bash",
		"--max-turns", strconv.Itoa(opts.MaxTurns),
	}

	slog.Info("running claude", "cwd", opts.CWD, "max_turns", opts.MaxTurns)

	cmd := exec.Command("claude", args...)
	cmd.Dir = opts.CWD
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("claude exited with error: %w", err)
	}
	return nil
}
