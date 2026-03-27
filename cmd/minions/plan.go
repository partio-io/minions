package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/partio-io/minions/internal/plan"
	"github.com/partio-io/minions/internal/program"
	"github.com/partio-io/minions/internal/prompt"
	"github.com/partio-io/minions/internal/propose"
	"github.com/partio-io/minions/internal/task"
)

func newPlanCmd() *cobra.Command {
	var (
		issueNum int
		repo     string
		dryRun   bool
	)

	cmd := &cobra.Command{
		Use:   "plan [task-file]",
		Short: "Generate an implementation plan for a task",
		Long: `Run Claude in plan mode (read-only) to generate an implementation plan.

The plan is posted as a comment on the corresponding GitHub issue.

Examples:
  minions plan tasks/my-task.yaml --dry-run
  minions plan --issue 42 --repo my-org/my-repo`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			if cfg.DryRun {
				dryRun = true
			}

			workspaceRoot := cfg.WorkspaceRoot
			if workspaceRoot == "" {
				wd, err := os.Getwd()
				if err != nil {
					return fmt.Errorf("getting working directory: %w", err)
				}
				workspaceRoot = filepath.Dir(wd)
			}

			// Load program from file or issue, convert to task for planning
			var t *task.Task
			if issueNum > 0 {
				if repo == "" {
					if proj == nil {
						return fmt.Errorf("project config required: ensure .minions/project.yaml exists or pass --repo")
					}
					repo = proj.PrincipalFullName()
				}
				prog, err := loadProgramFromIssue(repo, issueNum)
				if err != nil {
					return fmt.Errorf("loading program from issue #%d: %w", issueNum, err)
				}
				t = programToTask(prog)
			} else if len(args) > 0 {
				prog, err := program.LoadFile(args[0])
				if err != nil {
					return fmt.Errorf("loading program: %w", err)
				}
				t = programToTask(prog)
			} else {
				return fmt.Errorf("either a program file or --issue is required")
			}

			fmt.Println("==========================================")
			fmt.Println("PLAN MINION")
			fmt.Printf("TASK: %s\n", t.ID)
			fmt.Printf("TITLE: %s\n", t.Title)
			fmt.Println("==========================================")

			// Gather feedback from issue comments (for replan)
			var feedback, previousPlan string
			if issueNum > 0 {
				feedback, previousPlan = gatherReplanContext(repo, issueNum)
			}

			if dryRun {
				planPrompt, err := buildPlanPromptForDryRun(t, workspaceRoot, feedback, previousPlan)
				if err != nil {
					return err
				}
				fmt.Println()
				fmt.Println("=== DRY RUN: Plan Prompt ===")
				fmt.Println(planPrompt)
				fmt.Println("=== END PROMPT ===")
				return nil
			}

			// Generate plan
			fmt.Println("--- Generating plan ---")
			result, err := plan.Generate(ctx, plan.Opts{
				Task:          t,
				WorkspaceRoot: workspaceRoot,
				MaxTurns:      15,
				Feedback:      feedback,
				PreviousPlan:  previousPlan,
				Project:       proj,
			})
			if err != nil {
				return fmt.Errorf("plan generation failed: %w", err)
			}

			fmt.Printf("\nPlan generated (%d turns, $%.4f)\n", result.NumTurns, result.CostUSD)

			// Post to issue or print to stdout
			if issueNum > 0 {
				comment := formatPlanComment(result)
				if err := postPlanToIssue(repo, issueNum, comment); err != nil {
					return err
				}
				if err := updatePlanLabels(repo, issueNum); err != nil {
					slog.Warn("failed to update labels", "error", err)
				}
				fmt.Printf("Plan posted to %s#%d\n", repo, issueNum)
			} else {
				fmt.Println("\n" + formatPlanComment(result))
			}

			fmt.Println("\nDONE")
			return nil
		},
	}

	cmd.Flags().IntVar(&issueNum, "issue", 0, "GitHub issue number to plan for")
	cmd.Flags().StringVar(&repo, "repo", "", "GitHub repo (default: from project config)")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview generated prompt without executing")

	return cmd
}

// loadTaskFromIssue extracts the embedded task YAML from a GitHub issue.
func loadProgramFromIssue(repo string, issueNum int) (*program.Program, error) {
	cmd := exec.Command("gh", "issue", "view",
		fmt.Sprintf("%d", issueNum),
		"--repo", repo,
		"--json", "body",
		"-q", ".body",
	)
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("fetching issue: %w", err)
	}

	programMD := propose.ExtractProgramFromIssue(string(out))
	if programMD == "" {
		return nil, fmt.Errorf("no embedded program found in issue #%d", issueNum)
	}

	return program.Parse(programMD)
}

// gatherReplanContext fetches human comments and the previous plan from an issue.
func gatherReplanContext(repo string, issueNum int) (feedback, previousPlan string) {
	// Fetch issue comments
	cmd := exec.Command("gh", "api",
		fmt.Sprintf("repos/%s/issues/%d/comments", repo, issueNum),
		"--jq", ".",
	)
	out, err := cmd.Output()
	if err != nil {
		return "", ""
	}

	var comments []struct {
		Body string `json:"body"`
		User struct {
			Login string `json:"login"`
		} `json:"user"`
	}
	if err := json.Unmarshal(out, &comments); err != nil {
		return "", ""
	}

	// Find the latest plan comment and human feedback after it
	var lastPlanIdx int
	for i, c := range comments {
		if strings.Contains(c.Body, "## Minion Plan") {
			lastPlanIdx = i
			previousPlan = c.Body
		}
	}

	// Collect human comments after the last plan (excluding bot comments and commands)
	var feedbackParts []string
	for i := lastPlanIdx + 1; i < len(comments); i++ {
		c := comments[i]
		body := strings.TrimSpace(c.Body)
		// Skip bot comments and bare commands
		if strings.HasPrefix(body, "Minion ") || body == "/minion replan" {
			continue
		}
		// Strip the /minion replan command if present alongside feedback
		body = strings.ReplaceAll(body, "/minion replan", "")
		body = strings.TrimSpace(body)
		if body != "" {
			feedbackParts = append(feedbackParts, body)
		}
	}

	if len(feedbackParts) > 0 {
		feedback = strings.Join(feedbackParts, "\n\n")
	}

	return feedback, previousPlan
}

// formatPlanComment formats the plan result as a GitHub issue comment.
func formatPlanComment(result *plan.Result) string {
	var b strings.Builder

	b.WriteString("## Minion Plan\n\n")
	b.WriteString(result.Plan)
	b.WriteString("\n\n")

	if result.Questions != "" {
		b.WriteString("## Questions\n\n")
		b.WriteString(result.Questions)
		b.WriteString("\n\n")
	}

	b.WriteString("---\n\n")
	b.WriteString("Comment `/minion build` to execute this plan, or `/minion replan` with feedback to revise.\n")

	return b.String()
}

// postPlanToIssue posts a plan comment on a GitHub issue.
func postPlanToIssue(repo string, issueNum int, comment string) error {
	cmd := exec.Command("gh", "issue", "comment",
		fmt.Sprintf("%d", issueNum),
		"--repo", repo,
		"--body", comment,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("posting plan comment: %s: %w", strings.TrimSpace(string(out)), err)
	}
	return nil
}

// updatePlanLabels adds minion-planned and removes minion-planning.
func updatePlanLabels(repo string, issueNum int) error {
	issue := fmt.Sprintf("%d", issueNum)

	// Add minion-planned
	add := exec.Command("gh", "issue", "edit", issue, "--repo", repo, "--add-label", "minion-planned")
	if out, err := add.CombinedOutput(); err != nil {
		return fmt.Errorf("adding minion-planned label: %s: %w", strings.TrimSpace(string(out)), err)
	}

	// Remove minion-planning (may not exist, that's OK)
	remove := exec.Command("gh", "issue", "edit", issue, "--repo", repo, "--remove-label", "minion-planning")
	_ = remove.Run()

	return nil
}

// buildPlanPromptForDryRun generates the plan prompt for --dry-run output.
func buildPlanPromptForDryRun(t *task.Task, workspaceRoot, feedback, previousPlan string) (string, error) {
	return prompt.BuildPlan(t, workspaceRoot, feedback, previousPlan, proj)
}

// programToTask converts a Program to a Task for use with the planning system.
func programToTask(prog *program.Program) *task.Task {
	return &task.Task{
		ID:                 prog.ID,
		Title:              prog.Title,
		Source:             prog.Source,
		Description:        prog.Description,
		TargetRepos:        prog.TargetRepos,
		ContextHints:       prog.ContextHints,
		AcceptanceCriteria: prog.AcceptanceCriteria,
		PRLabels:           prog.PRLabels,
	}
}
