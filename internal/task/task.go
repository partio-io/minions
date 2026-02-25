package task

import "fmt"

// Task represents a minion task specification loaded from YAML.
type Task struct {
	ID                 string   `yaml:"id"`
	Title              string   `yaml:"title"`
	Source             string   `yaml:"source"`
	SourceType         string   `yaml:"source_type"`
	Description        string   `yaml:"description"`
	TargetRepos        []string `yaml:"target_repos"`
	ContextHints       []string `yaml:"context_hints"`
	AcceptanceCriteria []string `yaml:"acceptance_criteria"`
	PRLabels           []string `yaml:"pr_labels"`
	DependsOn          []string `yaml:"depends_on"`
}

// Validate checks that the task has all required fields.
func (t *Task) Validate() error {
	if t.ID == "" {
		return fmt.Errorf("task missing required field: id")
	}
	if t.Title == "" {
		return fmt.Errorf("task %q missing required field: title", t.ID)
	}
	if t.Description == "" {
		return fmt.Errorf("task %q missing required field: description", t.ID)
	}
	if len(t.TargetRepos) == 0 {
		return fmt.Errorf("task %q missing required field: target_repos", t.ID)
	}
	if len(t.AcceptanceCriteria) == 0 {
		return fmt.Errorf("task %q missing required field: acceptance_criteria", t.ID)
	}
	return nil
}
