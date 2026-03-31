package program

// Program is the parsed representation of an .md program file.
type Program struct {
	// From frontmatter
	ID                 string   `yaml:"id"`
	TargetRepos        []string `yaml:"target_repos"`
	PRLabels           []string `yaml:"pr_labels"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria"`
	DependsOn          []string `yaml:"depends_on"`
	Source             string   `yaml:"source"`

	// From markdown body
	Title       string // first H1
	Description string // text between H1 and first H2

	// From ## Context section
	ContextHints []string

	// From ## Planner section
	Planner *PhaseDef

	// From ## Agents section
	Agents []AgentDef

	// Raw source
	SourcePath string
	RawContent string
}

// PhaseDef configures the planner phase.
type PhaseDef struct {
	Tools          []string `yaml:"tools"`
	PermissionMode string   `yaml:"permission_mode"`
	MaxTurns       int      `yaml:"max_turns"`
	MaxBudgetUSD   float64  `yaml:"max_budget_usd"`
	MCPs           []MCPDef `yaml:"mcps"`
	Instructions   string   // prose after the capabilities block
}

// AgentDef configures a sub-agent for execution.
type AgentDef struct {
	Name          string   `yaml:"-"` // from H3 heading
	TargetRepos   []string `yaml:"target_repos"`
	Tools         []string `yaml:"tools"`
	MaxTurns      int      `yaml:"max_turns"`
	MaxBudgetUSD  float64  `yaml:"max_budget_usd"`
	Checks        bool     `yaml:"checks"`
	RetryOnFail   bool     `yaml:"retry_on_fail"`
	RetryMaxTurns int      `yaml:"retry_max_turns"`
	MCPs          []MCPDef `yaml:"mcps"`
	StageFiles    []string `yaml:"stage_files"`
	SkipMarker    string   `yaml:"skip_marker"`
	Instructions  string   // prose after the capabilities block
}

// MCPDef configures an MCP server for an agent.
type MCPDef struct {
	Name    string            `yaml:"name"`
	Type    string            `yaml:"type"` // "stdio", "sse", "http"
	Command string            `yaml:"command"`
	Args    []string          `yaml:"args"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers"`
	Env     map[string]string `yaml:"env"`
}
