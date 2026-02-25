package config

import (
	"os"
	"strconv"
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	WorkspaceRoot string
	MaxTurns      int
	DryRun        bool
	LogLevel      string
}

// Load reads configuration from environment variables with sensible defaults.
func Load() Config {
	c := Config{
		WorkspaceRoot: os.Getenv("WORKSPACE_ROOT"),
		MaxTurns:      30,
		LogLevel:      "info",
	}

	if v := os.Getenv("MINION_MAX_TURNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			c.MaxTurns = n
		}
	}

	if os.Getenv("MINION_DRY_RUN") == "1" {
		c.DryRun = true
	}

	if v := os.Getenv("MINION_LOG_LEVEL"); v != "" {
		c.LogLevel = v
	}

	return c
}
