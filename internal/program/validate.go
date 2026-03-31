package program

import "fmt"

// Validate checks that a Program has the minimum required fields.
func (p *Program) Validate() error {
	if p.ID == "" {
		return fmt.Errorf("program id is required")
	}
	if p.Title == "" {
		return fmt.Errorf("program title (H1 heading) is required")
	}
	return nil
}

// AllTargetRepos returns the union of program-level and agent-level target repos.
func (p *Program) AllTargetRepos() []string {
	seen := make(map[string]bool)
	var result []string

	for _, r := range p.TargetRepos {
		if !seen[r] {
			seen[r] = true
			result = append(result, r)
		}
	}
	for _, a := range p.Agents {
		for _, r := range a.TargetRepos {
			if !seen[r] {
				seen[r] = true
				result = append(result, r)
			}
		}
	}
	return result
}

// EffectiveTargetRepos returns the target repos for an agent,
// falling back to program-level repos if the agent doesn't override.
func (p *Program) EffectiveTargetRepos(a *AgentDef) []string {
	if len(a.TargetRepos) > 0 {
		return a.TargetRepos
	}
	return p.TargetRepos
}
