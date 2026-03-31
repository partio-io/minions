package program

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadFile parses an .md program file from disk.
func LoadFile(path string) (*Program, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading program file %s: %w", path, err)
	}
	p, err := Parse(string(data))
	if err != nil {
		return nil, fmt.Errorf("parsing program file %s: %w", path, err)
	}
	p.SourcePath = path
	return p, nil
}

// Parse parses an .md program from its raw content.
func Parse(content string) (*Program, error) {
	p := &Program{RawContent: content}

	// Split frontmatter and body
	body, err := parseFrontmatter(content, p)
	if err != nil {
		return nil, err
	}

	// Parse markdown sections
	if err := parseSections(body, p); err != nil {
		return nil, err
	}

	return p, nil
}

// parseFrontmatter extracts YAML frontmatter between --- delimiters.
func parseFrontmatter(content string, p *Program) (string, error) {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "---") {
		return content, nil // no frontmatter
	}

	rest := content[3:]
	fmContent, body, found := strings.Cut(rest, "\n---")
	if !found {
		return content, nil // unclosed frontmatter, treat as no frontmatter
	}

	if err := yaml.Unmarshal([]byte(fmContent), p); err != nil {
		return "", fmt.Errorf("parsing frontmatter: %w", err)
	}

	return body, nil
}

// section represents a parsed markdown section.
type section struct {
	level   int // heading level (1, 2, 3)
	heading string
	body    string
}

var headingRe = regexp.MustCompile(`^(#{1,6})\s+(.+)$`)

// parseSections walks the markdown body and populates the Program.
func parseSections(body string, p *Program) error {
	sections := splitSections(body)

	var inAgents bool
	for _, sec := range sections {
		switch {
		case sec.level == 1:
			p.Title = sec.heading
			p.Description = strings.TrimSpace(sec.body)

		case sec.level == 2 && strings.EqualFold(sec.heading, "Context"):
			p.ContextHints = parseContextHints(sec.body)

		case sec.level == 2 && strings.EqualFold(sec.heading, "Planner"):
			phase, err := parsePhaseDef(sec.body)
			if err != nil {
				return fmt.Errorf("parsing planner section: %w", err)
			}
			p.Planner = phase
			inAgents = false

		case sec.level == 2 && strings.EqualFold(sec.heading, "Agents"):
			inAgents = true

		case sec.level == 3 && inAgents:
			agent, err := parseAgentDef(sec.heading, sec.body)
			if err != nil {
				return fmt.Errorf("parsing agent %q: %w", sec.heading, err)
			}
			p.Agents = append(p.Agents, *agent)
		}
	}

	return nil
}

// splitSections splits markdown content into sections by heading.
func splitSections(body string) []section {
	lines := strings.Split(body, "\n")
	var sections []section
	var current *section

	for _, line := range lines {
		if m := headingRe.FindStringSubmatch(line); m != nil {
			// Save previous section
			if current != nil {
				sections = append(sections, *current)
			}
			current = &section{
				level:   len(m[1]),
				heading: strings.TrimSpace(m[2]),
			}
			continue
		}

		if current != nil {
			current.body += line + "\n"
		}
	}

	if current != nil {
		sections = append(sections, *current)
	}

	return sections
}

// parseContextHints extracts file paths from a bullet list.
func parseContextHints(body string) []string {
	var hints []string
	for line := range strings.SplitSeq(body, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") {
			hint := strings.TrimSpace(line[2:])
			// Strip inline code backticks
			hint = strings.Trim(hint, "`")
			if hint != "" {
				hints = append(hints, hint)
			}
		}
	}
	return hints
}

var capabilitiesBlockRe = regexp.MustCompile("(?s)```capabilities\\s*\n(.*?)```")

// extractCapabilitiesBlock extracts the capabilities code block and remaining prose.
func extractCapabilitiesBlock(body string) (capsYAML string, prose string) {
	loc := capabilitiesBlockRe.FindStringIndex(body)
	if loc == nil {
		return "", strings.TrimSpace(body)
	}

	match := capabilitiesBlockRe.FindStringSubmatch(body)
	capsYAML = match[1]

	// Remove the block from the body to get prose
	prose = body[:loc[0]] + body[loc[1]:]
	prose = strings.TrimSpace(prose)
	return capsYAML, prose
}

// capsIntermediate is used for YAML unmarshaling of capabilities blocks.
// The 'tools' field can be either a YAML list or a comma-separated string.
type capsIntermediate struct {
	Tools          yaml.Node `yaml:"tools"`
	TargetRepos    []string  `yaml:"target_repos"`
	PermissionMode string    `yaml:"permission_mode"`
	MaxTurns       int       `yaml:"max_turns"`
	MaxBudgetUSD   float64   `yaml:"max_budget_usd"`
	Checks         bool      `yaml:"checks"`
	RetryOnFail    bool      `yaml:"retry_on_fail"`
	RetryMaxTurns  int       `yaml:"retry_max_turns"`
	MCPs           []MCPDef  `yaml:"mcps"`
	StageFiles     []string  `yaml:"stage_files"`
	SkipMarker     string    `yaml:"skip_marker"`
}

// parseTools parses a tools YAML node which can be a scalar string (comma-separated)
// or a sequence of strings.
func parseTools(node yaml.Node) []string {
	switch node.Kind {
	case yaml.ScalarNode:
		raw := node.Value
		parts := strings.Split(raw, ",")
		tools := make([]string, 0, len(parts))
		for _, p := range parts {
			p = strings.TrimSpace(p)
			if p != "" {
				tools = append(tools, p)
			}
		}
		return tools
	case yaml.SequenceNode:
		tools := make([]string, 0, len(node.Content))
		for _, n := range node.Content {
			tools = append(tools, n.Value)
		}
		return tools
	default:
		return nil
	}
}

// parsePhaseDef parses a ## Planner section body into a PhaseDef.
func parsePhaseDef(body string) (*PhaseDef, error) {
	capsYAML, prose := extractCapabilitiesBlock(body)
	if capsYAML == "" {
		return &PhaseDef{Instructions: prose}, nil
	}

	var caps capsIntermediate
	if err := yaml.Unmarshal([]byte(capsYAML), &caps); err != nil {
		return nil, fmt.Errorf("parsing capabilities: %w", err)
	}

	return &PhaseDef{
		Tools:          parseTools(caps.Tools),
		PermissionMode: caps.PermissionMode,
		MaxTurns:       caps.MaxTurns,
		MaxBudgetUSD:   caps.MaxBudgetUSD,
		MCPs:           caps.MCPs,
		Instructions:   prose,
	}, nil
}

// parseAgentDef parses an ### Agent section into an AgentDef.
func parseAgentDef(heading, body string) (*AgentDef, error) {
	capsYAML, prose := extractCapabilitiesBlock(body)
	if capsYAML == "" {
		return &AgentDef{
			Name:         heading,
			Instructions: prose,
		}, nil
	}

	var caps capsIntermediate
	if err := yaml.Unmarshal([]byte(capsYAML), &caps); err != nil {
		return nil, fmt.Errorf("parsing capabilities: %w", err)
	}

	return &AgentDef{
		Name:          heading,
		TargetRepos:   caps.TargetRepos,
		Tools:         parseTools(caps.Tools),
		MaxTurns:      caps.MaxTurns,
		MaxBudgetUSD:  caps.MaxBudgetUSD,
		Checks:        caps.Checks,
		RetryOnFail:   caps.RetryOnFail,
		RetryMaxTurns: caps.RetryMaxTurns,
		MCPs:          caps.MCPs,
		StageFiles:    caps.StageFiles,
		SkipMarker:    caps.SkipMarker,
		Instructions:  prose,
	}, nil
}
