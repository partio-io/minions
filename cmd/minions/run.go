package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	appcontext "github.com/partio-io/minions/internal/context"
	"github.com/partio-io/minions/internal/executor"
	"github.com/partio-io/minions/internal/planner"
	"github.com/partio-io/minions/internal/program"
	"github.com/partio-io/minions/internal/slices"
	"github.com/partio-io/minions/internal/workspace"
)

func newRunCmd() *cobra.Command {
	var dryRun bool
	var issueRef string
	var prRef string

	cmd := &cobra.Command{
		Use:   "run <program.md>",
		Short: "Execute a program",
		Long: `Execute an .md program file, optionally with a GitHub issue or pull request as context.

Examples:
  minions run .minions/programs/implement.md --issue 120
  minions run .minions/programs/implement.md --issue partio-io/cli#120
  minions run .minions/programs/doc-update.md --pr partio-io/cli#488
  minions run .minions/programs/propose.md
  minions run .minions/programs/implement.md --issue 120 --dry-run`,
		Args: cobra.ExactArgs(1),
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

			// Fetch issue context if --issue is provided
			var issueContext, issueNumber string
			var issue issueView
			if issueRef != "" {
				var err error
				issue, issueNumber, err = fetchIssue(issueRef)
				if err != nil {
					return fmt.Errorf("fetching issue: %w", err)
				}
				issueContext = renderIssueContext(issue, maxCommentChars)
			}

			// Fetch PR context if --pr is provided (mutually exclusive with --issue)
			var prContext, prNumber string
			if prRef != "" {
				var err error
				prContext, prNumber, err = fetchPR(prRef)
				if err != nil {
					return fmt.Errorf("fetching PR: %w", err)
				}
			}

			return runProgram(ctx, args[0], workspaceRoot, issueContext, issueNumber, prContext, prNumber, issue, dryRun)
		},
	}

	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "preview generated prompt without executing")
	cmd.Flags().StringVar(&issueRef, "issue", "", "GitHub issue number or reference (e.g., 120 or org/repo#120)")
	cmd.Flags().StringVar(&prRef, "pr", "", "GitHub PR number or reference (e.g., 488 or org/repo#488); injects the PR title, body, and diff as context")
	cmd.MarkFlagsMutuallyExclusive("issue", "pr")

	return cmd
}

// parseRef splits a GitHub reference into repo and number. It accepts either a
// full reference (org/repo#123) or a bare number, in which case the principal
// repo from the project config is used. flagName only shapes the error message.
func parseRef(ref, flagName string) (repo, number string, err error) {
	if strings.Contains(ref, "#") {
		parts := strings.SplitN(ref, "#", 2)
		return parts[0], parts[1], nil
	}
	// Bare number — use principal repo from project config
	if proj == nil {
		return "", "", fmt.Errorf("--%s with a bare number requires project config (pass org/repo#number or ensure .minions/project.yaml exists)", flagName)
	}
	return proj.PrincipalFullName(), ref, nil
}

// maxCommentChars caps how much issue discussion is injected into the prompt.
// Threads grow without bound; prompts do not.
const maxCommentChars = 40000

// commentFramingCost approximates the per-comment heading and separators so the
// cap bounds the rendered text, not just the raw bodies.
const commentFramingCost = 64

// issueComment is one comment from `gh issue view --json comments`.
type issueComment struct {
	Author struct {
		Login string `json:"login"`
	} `json:"author"`
	Body        string `json:"body"`
	CreatedAt   string `json:"createdAt"`
	IsMinimized bool   `json:"isMinimized"`
}

// issueView is the subset of `gh issue view --json title,body,comments` we use.
type issueView struct {
	Title    string         `json:"title"`
	Body     string         `json:"body"`
	Comments []issueComment `json:"comments"`
}

// fetchIssue fetches an issue's title, body and discussion via gh CLI and
// returns the parsed view alongside the issue number. Accepts either a bare
// number (uses principal repo) or a full reference (org/repo#123). The view
// is returned structurally so the executor can detect slice-plan comments on
// raw bodies; callers render the prompt blob with renderIssueContext. The
// number is returned so the caller can make the per-build taskID/branch
// unique.
func fetchIssue(ref string) (issueView, string, error) {
	repo, number, err := parseRef(ref, "issue")
	if err != nil {
		return issueView{}, "", err
	}

	out, err := exec.Command("gh", "issue", "view", number, "--repo", repo, "--json", "title,body,comments").Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) && len(exitErr.Stderr) > 0 {
			return issueView{}, "", fmt.Errorf("gh issue view %s --repo %s: %w: %s",
				number, repo, err, strings.TrimSpace(string(exitErr.Stderr)))
		}
		return issueView{}, "", fmt.Errorf("gh issue view %s --repo %s: %w", number, repo, err)
	}

	var iv issueView
	if err := json.Unmarshal(out, &iv); err != nil {
		return issueView{}, "", fmt.Errorf("parsing gh issue view %s --repo %s: %w", number, repo, err)
	}

	return iv, number, nil
}

// toSliceComments converts gh-fetched comments to the structured form the
// executor consumes for slice-plan detection. Every comment passes through
// unfiltered — noise filtering stays a prompt-rendering concern.
func toSliceComments(comments []issueComment) []slices.Comment {
	if len(comments) == 0 {
		return nil
	}
	out := make([]slices.Comment, len(comments))
	for i, c := range comments {
		out[i] = slices.Comment{Author: c.Author.Login, Body: c.Body}
	}
	return out
}

// automatedCommentPrefixes open the comments minions posts about its own runs.
// They describe the machinery, not the work, so they only crowd out real
// discussion. Matching on the body is deliberate: the workflows comment with a
// human's token, so the author is indistinguishable from a person.
var automatedCommentPrefixes = []string{
	"Minion execution failed",
	"Minion completed",
}

// isNoiseComment reports whether a comment is machinery rather than discussion:
// a bare slash-command trigger, a minion status update, or a collapsed comment.
func isNoiseComment(c issueComment) bool {
	body := strings.TrimSpace(c.Body)
	if body == "" || c.IsMinimized {
		return true
	}
	// A trigger and nothing else — "/minion build".
	if strings.HasPrefix(body, "/minion") && !strings.Contains(body, "\n") {
		return true
	}
	for _, prefix := range automatedCommentPrefixes {
		if strings.HasPrefix(body, prefix) {
			return true
		}
	}
	return false
}

// selectComments marks the comments that fit within budget, working inward from
// both ends: the oldest carry the research and design intent, the newest carry
// course corrections, so the middle is what gets dropped. A comment too large
// to fit is skipped without blocking the smaller ones behind it.
func selectComments(comments []issueComment, budget int) (keep []bool, dropped int) {
	keep = make([]bool, len(comments))
	lo, hi := 0, len(comments)-1
	used := 0

	for head := true; lo <= hi; head = !head {
		i := hi
		if head {
			i = lo
		}
		if cost := len(comments[i].Body) + commentFramingCost; used+cost <= budget {
			used += cost
			keep[i] = true
		} else {
			dropped++
		}
		if head {
			lo++
		} else {
			hi--
		}
	}
	return keep, dropped
}

// renderIssueContext turns a fetched issue into the markdown handed to the
// agent. The discussion is included because that is where research, design
// decisions and scope boundaries live — an agent given only the body builds
// blind. Anything withheld is stated outright, since silent truncation reads
// as "you have everything".
func renderIssueContext(iv issueView, budget int) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n%s", iv.Title, strings.TrimSpace(iv.Body))

	var signal []issueComment
	for _, c := range iv.Comments {
		if !isNoiseComment(c) {
			signal = append(signal, c)
		}
	}
	if len(signal) == 0 {
		return strings.TrimSpace(b.String())
	}

	keep, dropped := selectComments(signal, budget)
	b.WriteString("\n\n---\n\n## Discussion\n\n")
	b.WriteString(discussionNote(len(iv.Comments), len(signal)-dropped, len(iv.Comments)-len(signal), dropped))

	gapNoted := false
	for i, c := range signal {
		if !keep[i] {
			if !gapNoted {
				fmt.Fprintf(&b, "\n\n_[%s omitted to fit the context budget]_", count(dropped, "comment"))
				gapNoted = true
			}
			continue
		}
		fmt.Fprintf(&b, "\n\n### @%s", c.Author.Login)
		if date, _, ok := strings.Cut(c.CreatedAt, "T"); ok {
			fmt.Fprintf(&b, " — %s", date)
		}
		fmt.Fprintf(&b, "\n\n%s", strings.TrimSpace(c.Body))
	}

	return strings.TrimSpace(b.String())
}

// discussionNote states plainly what the agent is, and is not, being shown.
func discussionNote(total, kept, filtered, dropped int) string {
	note := fmt.Sprintf("This issue has %s.", count(total, "comment"))
	if filtered > 0 {
		note += fmt.Sprintf(" %s of automated minion status were filtered out.", count(filtered, "comment"))
	}
	if dropped > 0 {
		note += fmt.Sprintf(" %s exceeded the context budget and %s omitted — assume the discussion below is incomplete.",
			count(dropped, "comment"), plural(dropped, "was", "were"))
	}
	if kept == 0 {
		return note
	}
	return note + fmt.Sprintf(" The remaining %s follow, oldest first.", count(kept, "comment"))
}

func count(n int, noun string) string {
	return fmt.Sprintf("%d %s", n, plural(n, noun, noun+"s"))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

// fetchPR fetches a pull request's title, body, and full diff via gh CLI and
// returns a context block alongside the parsed PR number. Accepts either a bare
// number (uses principal repo) or a full reference (org/repo#123). The diff is
// embedded so a program can document the change deterministically without a
// further network call, and the leading reference line tells the agent exactly
// which PR — and which repo — the change came from.
func fetchPR(ref string) (string, string, error) {
	repo, number, err := parseRef(ref, "pr")
	if err != nil {
		return "", "", err
	}

	meta, err := exec.Command("gh", "pr", "view", number, "--repo", repo, "--json", "title,body", "--jq", `"# " + .title + "\n\n" + .body`).Output()
	if err != nil {
		return "", "", fmt.Errorf("gh pr view %s --repo %s: %w", number, repo, err)
	}

	diff, err := exec.Command("gh", "pr", "diff", number, "--repo", repo).Output()
	if err != nil {
		return "", "", fmt.Errorf("gh pr diff %s --repo %s: %w", number, repo, err)
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Source pull request: %s#%s\n\n", repo, number)
	b.WriteString(strings.TrimSpace(string(meta)))
	b.WriteString("\n\n## Diff\n\n```diff\n")
	b.WriteString(strings.TrimSpace(string(diff)))
	b.WriteString("\n```")
	return b.String(), number, nil
}

func debugDirForTask(taskID string) string {
	base := os.Getenv("MINION_DEBUG_DIR")
	if base == "" {
		return ""
	}
	dir := filepath.Join(base, taskID)
	_ = os.MkdirAll(dir, 0755)
	return dir
}

func runProgram(ctx context.Context, programPath, workspaceRoot, issueContext, issueRef, prContext, prRef string, issue issueView, dryRun bool) error {
	prog, err := program.LoadFile(programPath)
	if err != nil {
		return err
	}
	if err := prog.Validate(); err != nil {
		return fmt.Errorf("invalid program: %w", err)
	}

	fmt.Println("==========================================")
	fmt.Printf("PROGRAM: %s\n", prog.ID)
	fmt.Printf("TITLE: %s\n", prog.Title)
	fmt.Printf("TARGET REPOS: %v\n", prog.AllTargetRepos())
	fmt.Printf("AGENTS: %d\n", len(prog.Agents))
	if issueContext != "" {
		fmt.Println("ISSUE: provided")
	}
	if prContext != "" {
		fmt.Println("PR: provided")
	}
	fmt.Printf("DRY RUN: %v\n", dryRun)
	fmt.Println("==========================================")

	if proj == nil {
		return fmt.Errorf("project config required: ensure .minions/project.yaml exists in the workspace")
	}

	allRepos := prog.AllTargetRepos()
	if err := workspace.EnsureRepos(proj, workspaceRoot, allRepos); err != nil {
		return fmt.Errorf("ensuring repos: %w", err)
	}

	tracker := appcontext.NewTracker(prog.ID)

	var planText string
	if prog.Planner != nil {
		fmt.Println("\n--- Planning Phase ---")
		planResult, err := planner.Run(ctx, planner.Opts{
			Program:       prog,
			WorkspaceRoot: workspaceRoot,
			Project:       proj,
			Tracker:       tracker,
			DryRun:        dryRun,
			DebugDir:      debugDirForTask(prog.ID),
		})
		if err != nil {
			return fmt.Errorf("planning failed: %w", err)
		}
		planText = planResult.Plan
		if planResult.PlanPath != "" {
			fmt.Printf("Plan saved to: %s\n", planResult.PlanPath)
		}
		if planResult.Questions != "" {
			fmt.Printf("\nPlanner questions:\n%s\n", planResult.Questions)
		}
	}

	if !dryRun {
		fmt.Println("\n--- Execution Phase ---")
	}
	result, err := executor.Run(ctx, executor.Opts{
		Program:       prog,
		PlanText:      planText,
		IssueContext:  issueContext,
		IssueTitle:    issue.Title,
		IssueBody:     issue.Body,
		IssueComments: toSliceComments(issue.Comments),
		IssueRef:      issueRef,
		PRContext:     prContext,
		PRRef:         prRef,
		WorkspaceRoot: workspaceRoot,
		Project:       proj,
		Tracker:       tracker,
		DryRun:        dryRun,
		DebugDir:      debugDirForTask(prog.ID),
	})
	if err != nil {
		return fmt.Errorf("execution failed: %w", err)
	}

	// Dry run stops here: prompts were printed by the executor, and the
	// sections below describe work (PRs, agent outcomes) that never ran.
	if dryRun {
		report := tracker.Report()
		report.PrintSummary()
		return nil
	}

	fmt.Println("\n==========================================")
	fmt.Printf("PROGRAM COMPLETE: %s\n", prog.ID)
	var allPRs []string
	var anyFailed bool
	for _, ar := range result.AgentResults {
		if ar.Error != nil {
			fmt.Printf("  FAILED: %s — %v\n", ar.AgentName, ar.Error)
			anyFailed = true
		} else if ar.Skipped {
			fmt.Printf("  SKIPPED: %s — %s\n", ar.AgentName, ar.SkipReason)
		} else {
			for _, url := range ar.PRURLs {
				fmt.Printf("  PR: %s (%s)\n", url, ar.AgentName)
				allPRs = append(allPRs, url)
			}
		}
	}
	fmt.Println("==========================================")

	report := tracker.Report()
	report.PrintSummary()

	if debugDir := debugDirForTask(prog.ID); debugDir != "" {
		_ = report.WriteJSON(filepath.Join(debugDir, "context-report.json"))
	}

	if anyFailed {
		return fmt.Errorf("one or more agents failed")
	}
	if len(allPRs) == 0 {
		slog.Warn("no PRs created by any agent")
	}
	return nil
}
