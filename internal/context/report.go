package context

import (
	"encoding/json"
	"fmt"
	"os"
)

// PrintSummary writes a human-readable summary to stdout.
func (r *ProgramReport) PrintSummary() {
	fmt.Println("--- Context Report ---")
	for _, p := range r.Phases {
		fmt.Printf("Phase: %s\n", p.Phase)
		fmt.Printf("  Prompt: %d chars (~%d tokens, %d components)\n",
			p.TotalChars, p.TotalTokens, len(p.InitialContext))
		for _, c := range p.InitialContext {
			fmt.Printf("    %-45s %6d chars  ~%5d tokens\n", c.Name, c.CharCount, c.TokenEstimate)
		}
		if p.NumTurns > 0 {
			fmt.Printf("  Result: %d input + %d output tokens, %d turns, %dms, $%.4f\n",
				p.InputTokens, p.OutputTokens, p.NumTurns, p.DurationMs, p.CostUSD)
			if p.CacheReadInputTokens > 0 {
				fmt.Printf("  Cache: %d creation + %d read tokens\n",
					p.CacheCreationInputTokens, p.CacheReadInputTokens)
			}
		}
		fmt.Println()
	}
	fmt.Printf("Total: $%.4f, %d turns, %dms\n", r.TotalCost, r.TotalTurns, r.TotalDuration)
	fmt.Println("--- End Context Report ---")
}

// WriteJSON writes the full report as JSON to a file.
func (r *ProgramReport) WriteJSON(path string) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling context report: %w", err)
	}
	return os.WriteFile(path, data, 0644)
}
