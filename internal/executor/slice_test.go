package executor

import (
	"bytes"
	gocontext "context"
	"io"
	"os"
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
		io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	w.Close()
	os.Stdout = old
	return <-done
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
