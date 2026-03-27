package context

// EstimateTokens returns a rough token count for a text string.
// Uses the standard chars/4 heuristic for Claude's tokenizer.
func EstimateTokens(text string) int {
	if len(text) == 0 {
		return 0
	}
	return (len(text) + 3) / 4 // ceiling division
}

// MeasureComponent creates a PromptComponent from a name and text content.
func MeasureComponent(name, content string) PromptComponent {
	return PromptComponent{
		Name:          name,
		CharCount:     len(content),
		TokenEstimate: EstimateTokens(content),
	}
}
