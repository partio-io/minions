package agent

import (
	"embed"
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	appcontext "github.com/partio-io/minions/internal/context"
	"github.com/partio-io/minions/internal/pipeline"
	"github.com/partio-io/minions/internal/prompt"
)

//go:embed agents/*
var agentDefs embed.FS

// Def describes an agent type loaded from YAML.
type Def struct {
	Name             string                  `yaml:"name"`
	Description      string                  `yaml:"description"`
	PromptTemplate   string                  `yaml:"prompt_template"`
	TargetRepos      []string                `yaml:"target_repos"`
	MaxTurns         int                     `yaml:"max_turns"`
	AllowedTools     string                  `yaml:"allowed_tools"`
	RunChecks        bool                    `yaml:"checks"`
	CreatePR         bool                    `yaml:"create_pr"`
	PRLabels         []string                `yaml:"pr_labels"`
	SkipMarker       string                  `yaml:"skip_marker"`
	RetryOnFail      bool                    `yaml:"retry_on_fail"`
	RetryMaxTurns    int                     `yaml:"retry_max_turns"`
	StageFiles       []string                `yaml:"stage_files"`
	ContextProviders []appcontext.ProviderDef `yaml:"context_providers"`
}

// Load loads an agent definition by name from the embedded agents directory.
func Load(name string) (*Def, error) {
	data, err := agentDefs.ReadFile("agents/" + name + ".yaml")
	if err != nil {
		return nil, fmt.Errorf("agent %q not found: %w", name, err)
	}

	var def Def
	if err := yaml.Unmarshal(data, &def); err != nil {
		return nil, fmt.Errorf("parsing agent %q: %w", name, err)
	}

	return &def, nil
}

// List returns the names of all available agent definitions.
func List() ([]string, error) {
	entries, err := agentDefs.ReadDir("agents")
	if err != nil {
		return nil, err
	}

	var names []string
	for _, e := range entries {
		name := e.Name()
		if strings.HasSuffix(name, ".yaml") {
			names = append(names, strings.TrimSuffix(name, ".yaml"))
		}
	}
	return names, nil
}

// BuildPipelineDef builds a pipeline.Def from an agent definition and context.
// The vars map provides template variables gathered by context providers.
// The opts provide runtime configuration (workspace root, task ID, dry run, etc.).
func BuildPipelineDef(agentDef *Def, vars map[string]string, opts PipelineOpts) (*pipeline.Def, error) {
	// Load and render the prompt template
	tmpl, err := prompt.Template(agentDef.PromptTemplate)
	if err != nil {
		return nil, fmt.Errorf("loading template %q: %w", agentDef.PromptTemplate, err)
	}

	promptText := RenderTemplate(tmpl, vars)

	// Determine target repos
	targetRepos := agentDef.TargetRepos
	if len(opts.TargetRepos) > 0 {
		targetRepos = opts.TargetRepos
	}

	def := &pipeline.Def{
		Name:          agentDef.Name,
		TaskID:        opts.TaskID,
		WorkspaceRoot: opts.WorkspaceRoot,
		TargetRepos:   targetRepos,
		PromptText:    promptText,
		MaxTurns:      agentDef.MaxTurns,
		AllowedTools:  agentDef.AllowedTools,
		SkipMarker:    agentDef.SkipMarker,
		RunChecks:     agentDef.RunChecks,
		RetryOnFail:   agentDef.RetryOnFail,
		RetryMaxTurns: agentDef.RetryMaxTurns,
		CreatePR:      agentDef.CreatePR,
		PRLabels:      agentDef.PRLabels,
		StageFiles:    agentDef.StageFiles,
		CommitMsg:     opts.CommitMsg,
		PRTitle:       opts.PRTitle,
		PRBody:        opts.PRBody,
		PRRepo:        opts.PRRepo,
		TaskTitle:     opts.TaskTitle,
		TaskDescription: opts.TaskDescription,
		TaskWhy:       opts.TaskWhy,
		SourcePRRepo:   opts.SourcePRRepo,
		SourcePRNumber: opts.SourcePRNumber,
		DryRun:        opts.DryRun,
		DebugDir:      opts.DebugDir,
	}

	// Override max turns if opts specifies one
	if opts.MaxTurns > 0 {
		def.MaxTurns = opts.MaxTurns
	}

	return def, nil
}

// PipelineOpts holds runtime options not part of the agent definition.
type PipelineOpts struct {
	TaskID        string
	WorkspaceRoot string
	TargetRepos   []string // override agent's target repos
	MaxTurns      int      // override agent's max turns (0 = use agent default)

	// PR fields (pre-computed by the caller)
	CommitMsg       string
	PRTitle         string
	PRBody          string
	PRRepo          string
	TaskTitle       string
	TaskDescription string
	TaskWhy         string
	SourcePRRepo    string
	SourcePRNumber  string

	DryRun   bool
	DebugDir string
}

// RenderTemplate substitutes {{VAR}} placeholders in a template with values from vars.
func RenderTemplate(tmpl string, vars map[string]string) string {
	result := tmpl
	for k, v := range vars {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}
