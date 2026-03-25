package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	claudesdk "github.com/partio-io/claude-agent-sdk-go"

	"github.com/partio-io/minions/internal/claude"
	"github.com/partio-io/minions/internal/prompt"

	"gopkg.in/yaml.v3"
)

// Feature represents a feature extracted by Claude from source content.
type Feature struct {
	ID                 string   `json:"id" yaml:"id"`
	Title              string   `json:"title" yaml:"title"`
	Source             string   `json:"source" yaml:"source"`
	Description        string   `json:"description" yaml:"description"`
	Why                string   `json:"why" yaml:"why"`
	UserRelevance      string   `json:"user_relevance" yaml:"user_relevance"`
	TargetRepos        []string `json:"target_repos" yaml:"target_repos"`
	ContextHints       []string `json:"context_hints" yaml:"context_hints"`
	AcceptanceCriteria []string `json:"acceptance_criteria" yaml:"acceptance_criteria"`
	Plan               bool     `json:"plan,omitempty" yaml:"plan,omitempty"`
}

// GenerateTasks sends content to Claude for feature extraction and writes task YAML files.
func GenerateTasks(ctx context.Context, sourceType, sourceURL, content, outputDir string) (int, error) {
	// Build the ingest prompt from template
	tmpl, err := prompt.Template("ingest-prompt.md")
	if err != nil {
		return 0, fmt.Errorf("loading ingest template: %w", err)
	}

	fullPrompt := tmpl
	fullPrompt = strings.ReplaceAll(fullPrompt, "{{SOURCE_TYPE}}", sourceType)
	fullPrompt = strings.ReplaceAll(fullPrompt, "{{SOURCE_URL}}", sourceURL)
	fullPrompt = strings.ReplaceAll(fullPrompt, "{{CONTENT}}", content)

	// Run Claude to extract features
	slog.Info("sending content to Claude for analysis", "source_type", sourceType, "content_len", len(content))

	resultMsg, err := claudesdk.Prompt(ctx, fullPrompt)
	if err != nil {
		return 0, fmt.Errorf("claude failed to extract features: %w", err)
	}
	if resultMsg.IsError || resultMsg.Result == nil {
		return 0, fmt.Errorf("claude returned error: subtype=%s", resultMsg.Subtype)
	}

	resultStr := claude.StripCodeFences(*resultMsg.Result)

	// Parse JSON array
	var features []Feature
	if err := json.Unmarshal([]byte(resultStr), &features); err != nil {
		return 0, fmt.Errorf("parsing features JSON: %w\nOutput: %s", err, resultStr)
	}

	if len(features) == 0 {
		slog.Info("no relevant features found")
		return 0, nil
	}

	slog.Info("found features", "count", len(features))

	// Write task YAML files
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return 0, fmt.Errorf("creating output directory: %w", err)
	}

	for _, f := range features {
		if err := writeTaskFile(f, sourceType, outputDir); err != nil {
			slog.Error("failed to write task file", "id", f.ID, "error", err)
			continue
		}
		slog.Info("created task file", "path", filepath.Join(outputDir, f.ID+".yaml"))
	}

	return len(features), nil
}

type taskFileData struct {
	ID                 string   `yaml:"id"`
	Title              string   `yaml:"title"`
	Source             string   `yaml:"source"`
	SourceType         string   `yaml:"source_type"`
	Description        string   `yaml:"description"`
	Why                string   `yaml:"why"`
	UserRelevance      string   `yaml:"user_relevance"`
	TargetRepos        []string `yaml:"target_repos"`
	ContextHints       []string `yaml:"context_hints"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria"`
	PRLabels           []string `yaml:"pr_labels"`
	Plan               bool     `yaml:"plan,omitempty"`
}

func writeTaskFile(f Feature, sourceType, outputDir string) error {
	data := taskFileData{
		ID:                 f.ID,
		Title:              f.Title,
		Source:             f.Source,
		SourceType:         sourceType,
		Description:        f.Description,
		Why:                f.Why,
		UserRelevance:      f.UserRelevance,
		TargetRepos:        f.TargetRepos,
		ContextHints:       f.ContextHints,
		AcceptanceCriteria: f.AcceptanceCriteria,
		PRLabels:           []string{"minion", "feature"},
		Plan:               f.Plan,
	}

	out, err := yaml.Marshal(&data)
	if err != nil {
		return fmt.Errorf("marshaling task YAML: %w", err)
	}

	path := filepath.Join(outputDir, f.ID+".yaml")
	return os.WriteFile(path, out, 0o644)
}
