package executor

import (
	"bytes"
	gocontext "context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pcontext "github.com/partio-io/minions/internal/context"
	"github.com/partio-io/minions/internal/program"
	"github.com/partio-io/minions/internal/slices"
)

const testPlanComment = `<!-- minion:research-slices parent=#42 -->

## Proposed slices

### Slice 1 — Plan parser

Parse the slice-plan comment into an ordered plan.

#### Acceptance criteria

- [ ] Publisher format parses
- [ ] Numbering gaps are rejected

### Slice 2 — Dry-run expansion

Expand the implement agent into one prompt per slice.

#### Acceptance criteria

- [ ] One prompt per slice is printed
`

const testPRDComment = `<!-- minion:research run-id=abc1234 -->

# Design

Use FindByBranch for resume detection.`

// captureStdout runs fn while capturing everything written to os.Stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	old := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = w
	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	_ = w.Close()
	os.Stdout = old
	return <-done
}

// captureLogs sends the default logger to a buffer, at every level, for
// the rest of the test, and restores the logger when the test ends.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	old := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(old) })
	return &buf
}

func sliceTestOpts(prog *program.Program, comments []slices.Comment) Opts {
	return Opts{
		Program:       prog,
		IssueTitle:    "Add a web UI for the reports dashboard",
		IssueBody:     "The dashboard needs a web UI so reports are browsable.",
		IssueContext:  "# Add a web UI for the reports dashboard\n\nThe dashboard needs a web UI so reports are browsable.",
		IssueRef:      "42",
		IssueComments: comments,
		WorkspaceRoot: "/nonexistent-workspace",
		Tracker:       pcontext.NewTracker("implement"),
		DryRun:        true,
	}
}

const malformedPlanComment = `<!-- minion:research-slices -->

### Slice 1 — Parser

#### Acceptance criteria

- One

### Slice 3 — Expansion

#### Acceptance criteria

- Two
`

func slicedProgram() *program.Program {
	return &program.Program{
		ID:          "implement",
		Title:       "Implement issue",
		Description: "Build what the issue asks for.",
		TargetRepos: []string{"api"},
		Slices:      true,
	}
}

func TestRun_MalformedPlan_FailsRun_NoFallback(t *testing.T) {
	comments := []slices.Comment{{Author: "jcleira", Body: malformedPlanComment}}

	var runErr error
	out := captureStdout(t, func() {
		_, runErr = Run(gocontext.Background(), sliceTestOpts(slicedProgram(), comments))
	})
	if runErr == nil {
		t.Fatal("Run: want parse error for malformed plan, got nil")
	}
	if !strings.Contains(runErr.Error(), "expected slice 2, found slice 3") {
		t.Errorf("error %q does not name the numbering gap", runErr)
	}
	if strings.Contains(out, "=== DRY RUN") {
		t.Errorf("prompts were printed despite malformed plan — fallback happened:\n%s", out)
	}
}

// assertSingleWholeIssuePrompt asserts today's non-slice dry-run output:
// exactly one prompt block, the unchanged header, the rendered issue blob,
// and no per-slice directive.
func assertSingleWholeIssuePrompt(t *testing.T, out string) {
	t.Helper()
	blocks := strings.Split(out, "=== DRY RUN:")[1:]
	if len(blocks) != 1 {
		t.Fatalf("want 1 prompt block, got %d\noutput:\n%s", len(blocks), out)
	}
	if !strings.HasPrefix(blocks[0], " Agent implement Prompt ===") {
		t.Errorf("single-prompt header changed: %q", strings.SplitN(blocks[0], "\n", 2)[0])
	}
	if !strings.Contains(blocks[0], "The dashboard needs a web UI") {
		t.Errorf("rendered issue context missing from prompt")
	}
	if strings.Contains(out, "Build only this slice") {
		t.Errorf("slice directive leaked into the whole-issue prompt")
	}
}

func TestRunDryRun_NoPlanComment_SingleWholeIssuePrompt(t *testing.T) {
	comments := []slices.Comment{
		{Author: "jcleira", Body: "just a discussion comment"},
		{Author: "jcleira", Body: testPRDComment},
	}

	var runErr error
	out := captureStdout(t, func() {
		_, runErr = Run(gocontext.Background(), sliceTestOpts(slicedProgram(), comments))
	})
	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}
	assertSingleWholeIssuePrompt(t, out)
}

func TestRunDryRun_UnflaggedProgram_IgnoresPlanComment(t *testing.T) {
	prog := slicedProgram()
	prog.Slices = false
	comments := []slices.Comment{{Author: "jcleira", Body: testPlanComment}}

	var runErr error
	out := captureStdout(t, func() {
		_, runErr = Run(gocontext.Background(), sliceTestOpts(prog, comments))
	})
	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}
	assertSingleWholeIssuePrompt(t, out)
}

func TestRunDryRun_SlicePlan_PrintsPerSlicePrompts(t *testing.T) {
	prog := slicedProgram()
	comments := []slices.Comment{
		{Author: "jcleira", Body: "just a discussion comment"},
		{Author: "jcleira", Body: testPRDComment},
		{Author: "jcleira", Body: testPlanComment},
	}

	var runErr error
	out := captureStdout(t, func() {
		_, runErr = Run(gocontext.Background(), sliceTestOpts(prog, comments))
	})
	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}

	blocks := strings.Split(out, "=== DRY RUN:")[1:]
	if len(blocks) != 2 {
		t.Fatalf("want 2 per-slice prompt blocks, got %d\noutput:\n%s", len(blocks), out)
	}

	directives := []string{
		"Build only this slice: Slice 1 — Plan parser",
		"Build only this slice: Slice 2 — Dry-run expansion",
	}
	for i, block := range blocks {
		if !strings.Contains(block, "The dashboard needs a web UI") {
			t.Errorf("block %d: missing issue body", i+1)
		}
		if !strings.Contains(block, "Use FindByBranch for resume detection.") {
			t.Errorf("block %d: missing PRD comment", i+1)
		}
		// Full plan present for orientation: both slice titles in every block.
		if !strings.Contains(block, "Plan parser") || !strings.Contains(block, "Dry-run expansion") {
			t.Errorf("block %d: missing full slice plan", i+1)
		}
		if !strings.Contains(block, directives[i]) {
			t.Errorf("block %d: missing directive %q", i+1, directives[i])
		}
	}
	if strings.Contains(blocks[0], directives[1]) {
		t.Errorf("block 1 carries slice 2's directive — slices not singled out")
	}
}

// seedEarlierSlice puts one completed slice on the run's branch in the
// workspace repo: a Go file declaring ident under dir, its work commit, and
// the slice marker. The base branch never carries the file.
func seedEarlierSlice(t *testing.T, ws, dir, ident string) {
	t.Helper()
	api := filepath.Join(ws, "api")
	liveGit(t, api, "checkout", "-q", "-b", liveBranch)
	if err := os.MkdirAll(filepath.Join(api, dir), 0o755); err != nil {
		t.Fatal(err)
	}
	src := "package " + dir + "\n\nfunc " + ident + "() {}\n"
	if err := os.WriteFile(filepath.Join(api, dir, dir+".go"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	liveGit(t, api, "add", "-A")
	liveGit(t, api, "commit", "-q", "-m", "slice 1/2: Plan parser")
	liveGit(t, api, "commit", "-q", "--allow-empty", "-m", slices.MarkerSubject(1, 2))
}

// TestRunDryRun_SlicePlan_ListsEarlierContributions is the tracer bullet for
// the "already built" section: a dry run against a workspace whose branch
// already carries slice one names slice one's declaration in slice two's
// prompt, and slice one's prompt has no such section.
func TestRunDryRun_SlicePlan_ListsEarlierContributions(t *testing.T) {
	ws, _ := setupSliceWorkspace(t)
	seedEarlierSlice(t, ws, "report", "ParseReport")

	prog := slicedProgram()
	comments := []slices.Comment{
		{Author: "jcleira", Body: testPRDComment},
		{Author: "jcleira", Body: testPlanComment},
	}
	opts := sliceTestOpts(prog, comments)
	opts.WorkspaceRoot = ws

	var runErr error
	out := captureStdout(t, func() {
		_, runErr = Run(gocontext.Background(), opts)
	})
	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}

	blocks := strings.Split(out, "=== DRY RUN:")[1:]
	if len(blocks) != 2 {
		t.Fatalf("want 2 per-slice prompt blocks, got %d\noutput:\n%s", len(blocks), out)
	}
	if strings.Contains(blocks[0], "## Already Built") {
		t.Errorf("slice 1 prompt carries an already-built section:\n%s", blocks[0])
	}
	if !strings.Contains(blocks[1], "## Already Built") {
		t.Fatalf("slice 2 prompt lacks the already-built section:\n%s", blocks[1])
	}
	for _, want := range []string{"ParseReport", "report/report.go", "slice 1"} {
		if !strings.Contains(blocks[1], want) {
			t.Errorf("slice 2 prompt: missing %q in already-built section:\n%s", want, blocks[1])
		}
	}
}

// TestRunDryRun_SlicePlan_UnreadableRepoDoesNotStopRun: a workspace whose
// repository cannot be read yields no contributions, and the run goes on.
// Slice two still gets the section, so the prompt shape does not depend on
// what the analysis found. The missing checkout is not the normal state of a
// new task, so the run warns about it.
func TestRunDryRun_SlicePlan_UnreadableRepoDoesNotStopRun(t *testing.T) {
	comments := []slices.Comment{{Author: "jcleira", Body: testPlanComment}}
	logs := captureLogs(t)

	var runErr error
	out := captureStdout(t, func() {
		_, runErr = Run(gocontext.Background(), sliceTestOpts(slicedProgram(), comments))
	})
	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}
	blocks := strings.Split(out, "=== DRY RUN:")[1:]
	if len(blocks) != 2 {
		t.Fatalf("want 2 per-slice prompt blocks, got %d\noutput:\n%s", len(blocks), out)
	}
	if !strings.Contains(blocks[1], "## Already Built") {
		t.Errorf("slice 2 prompt lacks the already-built section:\n%s", blocks[1])
	}
	if !strings.Contains(blocks[1], "No package-level Go declarations from the earlier slices were found") {
		t.Errorf("slice 2 prompt does not say that nothing was found:\n%s", blocks[1])
	}
	if !strings.Contains(logs.String(), "slice contributions: cannot determine origin's default branch") {
		t.Errorf("a missing checkout did not warn:\n%s", logs.String())
	}
}

// TestRunDryRun_SlicePlan_NewTaskLogsNoWarning: a dry run of a new task finds
// a workspace clone without the run's branch, because no slice has run yet.
// That is the normal case, so the run logs no warning about the branch, and
// the prompt of slice two still says that the earlier slices built nothing.
func TestRunDryRun_SlicePlan_NewTaskLogsNoWarning(t *testing.T) {
	ws, _ := setupSliceWorkspace(t)
	logs := captureLogs(t)

	opts := sliceTestOpts(slicedProgram(), []slices.Comment{{Author: "jcleira", Body: testPlanComment}})
	opts.WorkspaceRoot = ws

	var runErr error
	out := captureStdout(t, func() {
		_, runErr = Run(gocontext.Background(), opts)
	})
	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}
	blocks := strings.Split(out, "=== DRY RUN:")[1:]
	if len(blocks) != 2 {
		t.Fatalf("want 2 per-slice prompt blocks, got %d\noutput:\n%s", len(blocks), out)
	}
	if !strings.Contains(blocks[1], "No package-level Go declarations from the earlier slices were found") {
		t.Errorf("slice 2 prompt does not say that nothing was found:\n%s", blocks[1])
	}
	if strings.Contains(logs.String(), "slice contributions:") {
		t.Errorf("a dry run of a new task logged a branch warning:\n%s", logs.String())
	}
}

// TestRunDryRun_SlicePlan_CheckoutWithoutBranchIsSkippedAlone: in a
// multi-repo dry run, a checkout without the run's branch is left out on its
// own. The checkout that holds the branch still lists its earlier
// contributions under its repository prefix, as the live prompt does, and
// nothing warns.
func TestRunDryRun_SlicePlan_CheckoutWithoutBranchIsSkippedAlone(t *testing.T) {
	ws, _ := setupSliceWorkspace(t)
	seedEarlierSlice(t, ws, "report", "ParseReport")
	web := filepath.Join(ws, "web")
	liveGit(t, ws, "init", "-q", "-b", "main", web)
	liveGit(t, web, "commit", "-q", "--allow-empty", "-m", "initial")
	logs := captureLogs(t)

	prog := slicedProgram()
	prog.TargetRepos = []string{"api", "web"}
	opts := sliceTestOpts(prog, []slices.Comment{{Author: "jcleira", Body: testPlanComment}})
	opts.WorkspaceRoot = ws

	var runErr error
	out := captureStdout(t, func() {
		_, runErr = Run(gocontext.Background(), opts)
	})
	if runErr != nil {
		t.Fatalf("Run returned error: %v", runErr)
	}
	blocks := strings.Split(out, "=== DRY RUN:")[1:]
	if len(blocks) != 2 {
		t.Fatalf("want 2 per-slice prompt blocks, got %d\noutput:\n%s", len(blocks), out)
	}
	for _, want := range []string{"ParseReport", "api/report/report.go", "slice 1"} {
		if !strings.Contains(blocks[1], want) {
			t.Errorf("slice 2 prompt: missing %q in already-built section:\n%s", want, blocks[1])
		}
	}
	if strings.Contains(logs.String(), "slice contributions:") {
		t.Errorf("a checkout without the branch logged a warning:\n%s", logs.String())
	}
}
