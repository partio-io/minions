package claude

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"strings"

	claudesdk "github.com/partio-io/claude-agent-sdk-go"
)

// Opts configures a headless Claude Code invocation.
type Opts struct {
	Prompt         string
	CWD            string
	MaxTurns       int
	AllowedTools   string // Comma-separated tool list. Defaults to "Edit,Write,Read,Glob,Grep,Bash".
	LogFile        string // If set, write result JSON to this file for debug observability
	PermissionMode string // "default", "plan", "bypassPermissions", etc.
	MaxBudgetUSD   float64
	MCPServers     map[string]claudesdk.MCPServerConfig // MCP servers to register
}

// Result holds structured output from a Claude invocation.
type Result struct {
	Subtype      string  // "success", "error_max_turns", etc.
	ResultText   string
	NumTurns     int
	DurationMs   int
	DurationAPIMs int
	TotalCostUSD float64
	IsError      bool

	// Token usage from SDK ResultMessage.Usage
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
}

// Run executes a one-shot Claude prompt via the Agent SDK.
func Run(ctx context.Context, opts Opts) (*Result, error) {
	if opts.MaxTurns == 0 {
		opts.MaxTurns = 30
	}

	allowedTools := opts.AllowedTools
	if allowedTools == "" {
		allowedTools = "Edit,Write,Read,Glob,Grep,Bash"
	}

	tools := strings.Split(allowedTools, ",")

	sdkOpts := []claudesdk.Option{
		claudesdk.WithCwd(opts.CWD),
		claudesdk.WithMaxTurns(opts.MaxTurns),
		claudesdk.WithAllowedTools(tools...),
		claudesdk.WithVerbose(true),
	}

	if opts.PermissionMode != "" {
		sdkOpts = append(sdkOpts, claudesdk.WithPermissionMode(opts.PermissionMode))
	}
	if opts.MaxBudgetUSD > 0 {
		sdkOpts = append(sdkOpts, claudesdk.WithMaxBudgetUSD(opts.MaxBudgetUSD))
	}
	for name, srv := range opts.MCPServers {
		sdkOpts = append(sdkOpts, claudesdk.WithMCPServer(name, srv))
	}

	slog.Info("running claude", "cwd", opts.CWD, "max_turns", opts.MaxTurns)

	resultMsg, err := claudesdk.Prompt(ctx, opts.Prompt, sdkOpts...)
	if err != nil {
		return nil, fmt.Errorf("claude prompt failed: %w", err)
	}

	result := &Result{
		Subtype:       string(resultMsg.Subtype),
		NumTurns:      resultMsg.NumTurns,
		DurationMs:    resultMsg.DurationMs,
		DurationAPIMs: resultMsg.DurationAPIMs,
		IsError:       resultMsg.IsError,
	}
	if resultMsg.Result != nil {
		result.ResultText = *resultMsg.Result
	}
	if resultMsg.TotalCostUSD != nil {
		result.TotalCostUSD = *resultMsg.TotalCostUSD
	}
	if resultMsg.Usage != nil {
		result.InputTokens = resultMsg.Usage.InputTokens
		result.OutputTokens = resultMsg.Usage.OutputTokens
		result.CacheCreationInputTokens = resultMsg.Usage.CacheCreationInputTokens
		result.CacheReadInputTokens = resultMsg.Usage.CacheReadInputTokens
	}

	slog.Info("claude completed",
		"turns", result.NumTurns,
		"duration_ms", result.DurationMs,
		"cost_usd", result.TotalCostUSD,
		"subtype", result.Subtype,
	)

	if result.ResultText != "" {
		// Log first 500 chars of result text for debugging
		preview := result.ResultText
		if len(preview) > 500 {
			preview = preview[:500] + "..."
		}
		slog.Info("claude result text", "preview", preview)
	}

	// Write result to log file for debug observability
	if opts.LogFile != "" {
		if data, err := json.Marshal(resultMsg); err == nil {
			if err := os.WriteFile(opts.LogFile, data, 0644); err != nil {
				slog.Warn("failed to write claude log file", "path", opts.LogFile, "error", err)
			}
		}
	}

	return result, nil
}

// StripCodeFences removes ```json ... ``` wrapping if present.
func StripCodeFences(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "```") {
		if i := strings.Index(s, "\n"); i != -1 {
			s = s[i+1:]
		}
		s = strings.TrimSuffix(s, "```")
	}
	return strings.TrimSpace(s)
}
