package prompt

import (
	"fmt"
	"strings"

	"github.com/partio-io/minions/internal/project"
	"github.com/partio-io/minions/internal/task"
)

// BuildPlan constructs a plan-mode prompt from a task spec.
// If feedback and previousPlan are provided (replan), they are included as context.
// proj may be nil for backward compatibility.
func BuildPlan(t *task.Task, workspaceRoot, feedback, previousPlan string, proj *project.Project) (string, error) {
	tmpl, err := Template("plan-prompt.md")
	if err != nil {
		return "", fmt.Errorf("loading plan prompt template: %w", err)
	}

	// Reuse shared context builders
	reposText := buildTargetReposText(t, workspaceRoot, proj)
	criteriaText := buildAcceptanceCriteriaText(t)
	claudeMDText := buildClaudeMDText(t, workspaceRoot)
	hintsText := buildContextHintsText(t, workspaceRoot)

	// Build feedback section
	var feedbackSection string
	if feedback != "" || previousPlan != "" {
		var fb strings.Builder
		fb.WriteString("## Previous Plan & Feedback\n\n")
		if previousPlan != "" {
			fb.WriteString("### Previous Plan\n\n")
			fb.WriteString(previousPlan)
			fb.WriteString("\n\n")
		}
		if feedback != "" {
			fb.WriteString("### Human Feedback\n\n")
			fb.WriteString(feedback)
			fb.WriteString("\n\n")
		}
		fb.WriteString("Revise the plan based on the feedback above.\n")
		feedbackSection = fb.String()
	}

	result := tmpl
	result = strings.ReplaceAll(result, "{{TITLE}}", t.Title)
	result = strings.ReplaceAll(result, "{{DESCRIPTION}}", t.Description)
	result = strings.ReplaceAll(result, "{{TARGET_REPOS}}", reposText)
	result = strings.ReplaceAll(result, "{{ACCEPTANCE_CRITERIA}}", criteriaText)
	result = strings.ReplaceAll(result, "{{CLAUDE_MD_CONTENTS}}", claudeMDText)
	result = strings.ReplaceAll(result, "{{CONTEXT_HINTS_CONTENTS}}", hintsText)
	result = strings.ReplaceAll(result, "{{FEEDBACK_SECTION}}", feedbackSection)

	return result, nil
}
