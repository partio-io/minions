package context

import "time"

// PromptComponent describes one piece of the assembled prompt.
type PromptComponent struct {
	Name          string `json:"name"`
	CharCount     int    `json:"char_count"`
	TokenEstimate int    `json:"token_estimate"`
}

// PhaseReport tracks context usage for a single execution phase.
type PhaseReport struct {
	Phase     string    `json:"phase"` // "planner", "agent:cli-agent", etc.
	StartedAt time.Time `json:"started_at"`

	// Initial context loaded
	InitialContext []PromptComponent `json:"initial_context"`
	TotalChars     int               `json:"total_chars"`
	TotalTokens    int               `json:"total_tokens_estimate"`

	// Actual usage from SDK (populated after invocation)
	InputTokens              int     `json:"input_tokens"`
	OutputTokens             int     `json:"output_tokens"`
	CacheCreationInputTokens int     `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int     `json:"cache_read_input_tokens"`
	NumTurns                 int     `json:"num_turns"`
	DurationMs               int     `json:"duration_ms"`
	DurationAPIMs            int     `json:"duration_api_ms"`
	CostUSD                  float64 `json:"cost_usd"`
}

// ProgramReport aggregates all phases for a program execution.
type ProgramReport struct {
	ProgramID     string        `json:"program_id"`
	Phases        []PhaseReport `json:"phases"`
	TotalCost     float64       `json:"total_cost_usd"`
	TotalTurns    int           `json:"total_turns"`
	TotalDuration int           `json:"total_duration_ms"`
}

// Tracker accumulates reports across phases of a program.
type Tracker struct {
	ProgramID string
	phases    []PhaseReport
}

// NewTracker creates a tracker for a program execution.
func NewTracker(programID string) *Tracker {
	return &Tracker{ProgramID: programID}
}

// StartPhase begins tracking a new phase.
func (t *Tracker) StartPhase(phase string) *PhaseTracker {
	return &PhaseTracker{
		tracker: t,
		report: PhaseReport{
			Phase:     phase,
			StartedAt: time.Now(),
		},
	}
}

// Report returns the finalized ProgramReport.
func (t *Tracker) Report() *ProgramReport {
	r := &ProgramReport{
		ProgramID: t.ProgramID,
		Phases:    t.phases,
	}
	for _, p := range t.phases {
		r.TotalCost += p.CostUSD
		r.TotalTurns += p.NumTurns
		r.TotalDuration += p.DurationMs
	}
	return r
}

// PhaseTracker tracks a single phase in progress.
type PhaseTracker struct {
	tracker    *Tracker
	report     PhaseReport
	totalChars int
}

// AddContext records an initial context entry.
func (pt *PhaseTracker) AddContext(name, content string) {
	comp := MeasureComponent(name, content)
	pt.report.InitialContext = append(pt.report.InitialContext, comp)
	pt.totalChars += comp.CharCount
}

// Finish records the results and completes the phase.
func (pt *PhaseTracker) Finish(result *InvocationMetrics) {
	pt.report.TotalChars = pt.totalChars
	pt.report.TotalTokens = EstimateTokens("")
	// Recompute total tokens from components
	total := 0
	for _, c := range pt.report.InitialContext {
		total += c.TokenEstimate
	}
	pt.report.TotalTokens = total

	if result != nil {
		pt.report.InputTokens = result.InputTokens
		pt.report.OutputTokens = result.OutputTokens
		pt.report.CacheCreationInputTokens = result.CacheCreationInputTokens
		pt.report.CacheReadInputTokens = result.CacheReadInputTokens
		pt.report.NumTurns = result.NumTurns
		pt.report.DurationMs = result.DurationMs
		pt.report.DurationAPIMs = result.DurationAPIMs
		pt.report.CostUSD = result.CostUSD
	}

	pt.tracker.phases = append(pt.tracker.phases, pt.report)
}

// InvocationMetrics holds the metrics extracted from a Claude result.
// This is the interface between the context tracker and the claude package.
type InvocationMetrics struct {
	InputTokens              int
	OutputTokens             int
	CacheCreationInputTokens int
	CacheReadInputTokens     int
	NumTurns                 int
	DurationMs               int
	DurationAPIMs            int
	CostUSD                  float64
}
