package executor

import (
	gocontext "context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/partio-io/minions/internal/claude"
	"github.com/partio-io/minions/internal/pr"
	"github.com/partio-io/minions/internal/project"
	"github.com/partio-io/minions/internal/slices"
)

// liveBranch is the branch the live slice loop must reuse across every slice:
// identical to the single-session path's naming so downstream workflow
// lookups by head branch keep working.
const liveBranch = "minion/implement-implement-42"

// liveGit runs a git command in dir and fails the test on error.
func liveGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t",
		"GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return strings.TrimSpace(string(out))
}

// setupSliceWorkspace builds a workspace holding one repo "api" cloned from a
// local bare origin, mirroring the runner's persistent-checkout layout.
func setupSliceWorkspace(t *testing.T) (workspaceRoot, originDir string) {
	t.Helper()
	root := t.TempDir()
	originDir = filepath.Join(root, "origin.git")
	workspaceRoot = filepath.Join(root, "ws")
	api := filepath.Join(workspaceRoot, "api")

	liveGit(t, root, "init", "--bare", "-b", "main", originDir)
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	liveGit(t, root, "clone", "-q", originDir, api)
	if err := os.WriteFile(filepath.Join(api, "README.md"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	liveGit(t, api, "add", "-A")
	liveGit(t, api, "commit", "-q", "-m", "initial")
	liveGit(t, api, "push", "-q", "origin", "main")
	return workspaceRoot, originDir
}

// stubSeams snapshots the executor seams and restores them at test end.
// Comment posting is stubbed to a no-op up front so no test can ever reach
// the real gh CLI; tests asserting on posts install their own recorder.
func stubSeams(t *testing.T) {
	t.Helper()
	origClaude, origChecks, origPR, origPRURL, origPost := claudeRun, checksRun, prCreateAndLinkAll, prURLForBranch, postIssueComment
	postIssueComment = func(string, string, string) error { return nil }
	t.Cleanup(func() {
		claudeRun, checksRun, prCreateAndLinkAll, prURLForBranch, postIssueComment = origClaude, origChecks, origPR, origPRURL, origPost
	})
}

// liveSliceOpts returns live-run Opts for the two-slice test plan against the
// given workspace.
func liveSliceOpts(ws string) Opts {
	opts := sliceTestOpts(slicedProgram(), []slices.Comment{
		{Author: "jcleira", Body: testPRDComment},
		{Author: "jcleira", Body: testPlanComment},
	})
	opts.DryRun = false
	opts.WorkspaceRoot = ws
	opts.Project = &project.Project{
		Principal: project.RepoRef{Name: "api", FullName: "acme/api"},
		Repos:     []project.RepoEntry{{Name: "api", FullName: "acme/api"}},
	}
	return opts
}

// TestRun_SliceLoop_AdvancesPerSliceAndCreatesPROnlyAfterFinal is the tracer
// bullet for the live slice loop: two slices build on one shared branch, each
// in a fresh session and a fresh worktree at the branch tip, markers land on
// origin after each slice, and the PR is created exactly once at the end.
func TestRun_SliceLoop_AdvancesPerSliceAndCreatesPROnlyAfterFinal(t *testing.T) {
	ws, origin := setupSliceWorkspace(t)
	stubSeams(t)

	type workSession struct {
		prompt  string
		branch  string
		sawPrev bool // slice 1's file already present (committed) in this session's worktree
	}
	var work []workSession
	summaries := 0
	claudeRun = func(_ gocontext.Context, o claude.Opts) (*claude.Result, error) {
		if !strings.Contains(o.Prompt, "Build only this slice") {
			summaries++
			return &claude.Result{ResultText: "TITLE: Built it\n\nDESCRIPTION:\ndone"}, nil
		}
		n := len(work) + 1
		branch := liveGit(t, o.CWD, "rev-parse", "--abbrev-ref", "HEAD")
		_, statErr := os.Stat(filepath.Join(o.CWD, "slice-1.txt"))
		if err := os.WriteFile(filepath.Join(o.CWD, fmt.Sprintf("slice-%d.txt", n)), []byte("work"), 0o644); err != nil {
			t.Fatal(err)
		}
		work = append(work, workSession{prompt: o.Prompt, branch: branch, sawPrev: statErr == nil})
		return &claude.Result{}, nil
	}
	checksRun = func(string) (string, error) { return "", nil }

	type prCall struct {
		taskID    string
		originLog string // origin's branch subjects at PR-creation time
	}
	var prCalls []prCall
	prCreateAndLinkAll = func(taskID, _, _, _, _, _ string, _ []string, _ pr.FullNameFunc, _ string, _ *pr.CreateOpts) ([]string, error) {
		prCalls = append(prCalls, prCall{
			taskID:    taskID,
			originLog: liveGit(t, origin, "log", liveBranch, "--format=%s"),
		})
		return []string{"https://github.com/acme/api/pull/7"}, nil
	}

	res, err := Run(gocontext.Background(), liveSliceOpts(ws))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(res.AgentResults) != 1 {
		t.Fatalf("want 1 agent result, got %d", len(res.AgentResults))
	}
	ar := res.AgentResults[0]
	if ar.Error != nil {
		t.Fatalf("agent error: %v", ar.Error)
	}
	if len(ar.PRURLs) != 1 {
		t.Errorf("PR URLs = %v; want exactly one", ar.PRURLs)
	}

	if len(work) != 2 {
		t.Fatalf("want 2 slice sessions, got %d", len(work))
	}
	if !strings.Contains(work[0].prompt, "Build only this slice: Slice 1 — Plan parser") {
		t.Errorf("session 1 prompt lacks slice 1 directive")
	}
	if !strings.Contains(work[1].prompt, "Build only this slice: Slice 2 — Dry-run expansion") {
		t.Errorf("session 2 prompt lacks slice 2 directive")
	}
	for i, s := range work {
		if s.branch != liveBranch {
			t.Errorf("session %d ran on branch %q; want %q", i+1, s.branch, liveBranch)
		}
	}
	if !work[1].sawPrev {
		t.Errorf("slice 2's worktree lacks slice 1's committed file — slice N+1 does not see slice N's commits")
	}

	originLog := liveGit(t, origin, "log", liveBranch, "--format=%s")
	for _, marker := range []string{"minion:slice 1/2", "minion:slice 2/2"} {
		if !strings.Contains(originLog, marker) {
			t.Errorf("origin branch missing marker commit %q:\n%s", marker, originLog)
		}
	}

	if len(prCalls) != 1 {
		t.Fatalf("PR creation called %d times; want exactly 1", len(prCalls))
	}
	if prCalls[0].taskID != "implement-implement-42" {
		t.Errorf("PR taskID = %q; want implement-implement-42", prCalls[0].taskID)
	}
	if !strings.Contains(prCalls[0].originLog, "minion:slice 2/2") {
		t.Errorf("PR was created before the final slice's marker reached origin")
	}
	if summaries != 1 {
		t.Errorf("summarize sessions = %d; want 1", summaries)
	}
}

// seedOriginBranch pushes the minion branch to origin from a throwaway clone,
// simulating a prior run's pushed state. The workspace clone never sees the
// branch locally — resume must come from fetching origin. subjects are commit
// messages, oldest first; "file:" entries write that file as slice work.
func seedOriginBranch(t *testing.T, originDir string, subjects []string) {
	t.Helper()
	seed := filepath.Join(t.TempDir(), "seed")
	liveGit(t, t.TempDir(), "clone", "-q", originDir, seed)
	liveGit(t, seed, "checkout", "-q", "-b", liveBranch)
	for _, s := range subjects {
		if name, ok := strings.CutPrefix(s, "file:"); ok {
			if err := os.WriteFile(filepath.Join(seed, name), []byte("prior work"), 0o644); err != nil {
				t.Fatal(err)
			}
			liveGit(t, seed, "add", "-A")
			liveGit(t, seed, "commit", "-q", "-m", "slice work: "+name)
			continue
		}
		liveGit(t, seed, "commit", "-q", "--allow-empty", "-m", s)
	}
	liveGit(t, seed, "push", "-q", "origin", liveBranch)
}

// TestRun_SliceLoop_ResumeSkipsCompletedSlices is the tracer bullet for
// resume: origin already holds the minion branch with slice 1's work and its
// completion marker. The run must fetch that branch, run no session for slice
// 1, build slice 2 in a worktree carrying slice 1's pushed file, and create
// the PR once.
func TestRun_SliceLoop_ResumeSkipsCompletedSlices(t *testing.T) {
	ws, origin := setupSliceWorkspace(t)
	seedOriginBranch(t, origin, []string{"file:slice-1.txt", "minion:slice 1/2"})
	stubSeams(t)

	type workSession struct {
		prompt  string
		branch  string
		sawPrev bool // slice 1's pushed file present in this session's worktree
	}
	var work []workSession
	summaries := 0
	claudeRun = func(_ gocontext.Context, o claude.Opts) (*claude.Result, error) {
		if !strings.Contains(o.Prompt, "Build only this slice") {
			summaries++
			return &claude.Result{ResultText: "TITLE: t\n\nDESCRIPTION:\nd"}, nil
		}
		branch := liveGit(t, o.CWD, "rev-parse", "--abbrev-ref", "HEAD")
		_, statErr := os.Stat(filepath.Join(o.CWD, "slice-1.txt"))
		if err := os.WriteFile(filepath.Join(o.CWD, "slice-2.txt"), []byte("work"), 0o644); err != nil {
			t.Fatal(err)
		}
		work = append(work, workSession{prompt: o.Prompt, branch: branch, sawPrev: statErr == nil})
		return &claude.Result{}, nil
	}
	checksRun = func(string) (string, error) { return "", nil }

	prCalled := 0
	prCreateAndLinkAll = func(string, string, string, string, string, string, []string, pr.FullNameFunc, string, *pr.CreateOpts) ([]string, error) {
		prCalled++
		return []string{"https://github.com/acme/api/pull/7"}, nil
	}

	res, err := Run(gocontext.Background(), liveSliceOpts(ws))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	ar := res.AgentResults[0]
	if ar.Error != nil {
		t.Fatalf("agent error: %v", ar.Error)
	}

	if len(work) != 1 {
		t.Fatalf("want 1 slice session (slice 1 already complete), got %d", len(work))
	}
	if !strings.Contains(work[0].prompt, "Build only this slice: Slice 2 — Dry-run expansion") {
		t.Errorf("resumed session prompt lacks slice 2 directive:\n%s", work[0].prompt)
	}
	if work[0].branch != liveBranch {
		t.Errorf("resumed session ran on branch %q; want %q", work[0].branch, liveBranch)
	}
	if !work[0].sawPrev {
		t.Errorf("resumed worktree lacks slice 1's pushed file — not checked out from the fetched branch")
	}

	originLog := liveGit(t, origin, "log", liveBranch, "--format=%s")
	if got := strings.Count(originLog, "minion:slice 1/2"); got != 1 {
		t.Errorf("origin has %d slice-1 markers; want 1 (completed slice must not re-run):\n%s", got, originLog)
	}
	if got := strings.Count(originLog, "minion:slice 2/2"); got != 1 {
		t.Errorf("origin has %d slice-2 markers; want 1:\n%s", got, originLog)
	}
	if !strings.Contains(originLog, "slice work: slice-1.txt") {
		t.Errorf("seeded slice 1 work missing from origin — resume rebuilt the branch instead of extending it:\n%s", originLog)
	}
	if prCalled != 1 {
		t.Errorf("PR creation called %d times; want 1", prCalled)
	}
	if summaries != 1 {
		t.Errorf("summarize sessions = %d; want 1", summaries)
	}
}

// seedOriginMain lands commits on origin's default branch, standing in for
// minion work that earlier runs already merged.
func seedOriginMain(t *testing.T, originDir string, subjects []string) {
	t.Helper()
	seed := filepath.Join(t.TempDir(), "seed-main")
	liveGit(t, t.TempDir(), "clone", "-q", originDir, seed)
	for _, s := range subjects {
		liveGit(t, seed, "commit", "-q", "--allow-empty", "-m", s)
	}
	liveGit(t, seed, "push", "-q", "origin", "main")
}

// TestRun_SliceLoop_ResumeIgnoresMarkersInheritedFromBase is the regression
// guard for the count that broke every resume in a repo with merged minion
// history. Main carries three markers from earlier runs and the minion branch
// carries one of its own. Counting from the branch tip sees four against a
// two-slice plan and aborts; counting the branch's own commits sees one and
// resumes at slice 2.
func TestRun_SliceLoop_ResumeIgnoresMarkersInheritedFromBase(t *testing.T) {
	ws, origin := setupSliceWorkspace(t)
	seedOriginMain(t, origin, []string{"minion:slice 1/3", "minion:slice 2/3", "minion:slice 3/3"})
	seedOriginBranch(t, origin, []string{"file:slice-1.txt", "minion:slice 1/2"})
	stubSeams(t)

	var prompts []string
	claudeRun = func(_ gocontext.Context, o claude.Opts) (*claude.Result, error) {
		if !strings.Contains(o.Prompt, "Build only this slice") {
			return &claude.Result{ResultText: "TITLE: t\n\nDESCRIPTION:\nd"}, nil
		}
		if err := os.WriteFile(filepath.Join(o.CWD, "slice-2.txt"), []byte("work"), 0o644); err != nil {
			t.Fatal(err)
		}
		prompts = append(prompts, o.Prompt)
		return &claude.Result{}, nil
	}
	checksRun = func(string) (string, error) { return "", nil }
	prCreateAndLinkAll = func(string, string, string, string, string, string, []string, pr.FullNameFunc, string, *pr.CreateOpts) ([]string, error) {
		return []string{"https://github.com/acme/api/pull/7"}, nil
	}

	res, err := Run(gocontext.Background(), liveSliceOpts(ws))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ar := res.AgentResults[0]; ar.Error != nil {
		t.Fatalf("agent error: %v — markers merged into main must not count against the plan", ar.Error)
	}
	if len(prompts) != 1 {
		t.Fatalf("want 1 slice session (slice 1 already complete), got %d", len(prompts))
	}
	if !strings.Contains(prompts[0], "Build only this slice: Slice 2 — Dry-run expansion") {
		t.Errorf("resumed session did not target slice 2:\n%s", prompts[0])
	}
}

// TestRun_SliceLoop_ResumeIgnoresLocalOnlyBranchState covers the
// never-trust-local rule: the workspace clone carries a local minion branch
// claiming every slice is done, but origin only has slice 1's marker. Resume
// must fetch and believe origin — run slice 2, on origin's tip — not exit
// early on the local lie.
func TestRun_SliceLoop_ResumeIgnoresLocalOnlyBranchState(t *testing.T) {
	ws, origin := setupSliceWorkspace(t)
	seedOriginBranch(t, origin, []string{"file:slice-1.txt", "minion:slice 1/2"})

	// Local-only lie in the workspace clone: both markers, no slice work,
	// never pushed. Switch back to main so the branch is free to be reset.
	api := filepath.Join(ws, "api")
	liveGit(t, api, "checkout", "-q", "-b", liveBranch)
	liveGit(t, api, "commit", "-q", "--allow-empty", "-m", "minion:slice 1/2")
	liveGit(t, api, "commit", "-q", "--allow-empty", "-m", "minion:slice 2/2")
	liveGit(t, api, "checkout", "-q", "main")

	stubSeams(t)

	type workSession struct {
		prompt  string
		sawPrev bool
	}
	var work []workSession
	claudeRun = func(_ gocontext.Context, o claude.Opts) (*claude.Result, error) {
		if !strings.Contains(o.Prompt, "Build only this slice") {
			return &claude.Result{ResultText: "TITLE: t\n\nDESCRIPTION:\nd"}, nil
		}
		_, statErr := os.Stat(filepath.Join(o.CWD, "slice-1.txt"))
		if err := os.WriteFile(filepath.Join(o.CWD, "slice-2.txt"), []byte("work"), 0o644); err != nil {
			t.Fatal(err)
		}
		work = append(work, workSession{prompt: o.Prompt, sawPrev: statErr == nil})
		return &claude.Result{}, nil
	}
	checksRun = func(string) (string, error) { return "", nil }
	prCreateAndLinkAll = func(string, string, string, string, string, string, []string, pr.FullNameFunc, string, *pr.CreateOpts) ([]string, error) {
		return []string{"https://github.com/acme/api/pull/7"}, nil
	}

	res, err := Run(gocontext.Background(), liveSliceOpts(ws))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ar := res.AgentResults[0]; ar.Error != nil {
		t.Fatalf("agent error: %v", ar.Error)
	}

	if len(work) != 1 {
		t.Fatalf("want 1 slice session (origin says 1 of 2 done), got %d — local-only state was trusted", len(work))
	}
	if !strings.Contains(work[0].prompt, "Build only this slice: Slice 2") {
		t.Errorf("session is not slice 2's build session:\n%s", work[0].prompt)
	}
	if !work[0].sawPrev {
		t.Errorf("slice 2's worktree lacks origin's slice-1 file — resumed from the local lie, not the fetched branch")
	}

	originLog := liveGit(t, origin, "log", liveBranch, "--format=%s")
	if got := strings.Count(originLog, "minion:slice 2/2"); got != 1 {
		t.Errorf("origin has %d slice-2 markers; want 1:\n%s", got, originLog)
	}
}

// allCompleteSeed is a fully built two-slice branch: both slices' work and
// both completion markers, as a run that died right before PR creation (or a
// plain re-trigger of a finished build) would have left origin.
var allCompleteSeed = []string{
	"file:slice-1.txt", "minion:slice 1/2",
	"file:slice-2.txt", "minion:slice 2/2",
}

// TestRun_SliceLoop_AllCompleteCreatesMissingPRWithoutSessions covers the
// died-between-push-and-PR re-run: every slice is marked complete on origin
// but no PR exists. The run must create the PR from a worktree on the fetched
// branch — no build session, no new marker commits.
func TestRun_SliceLoop_AllCompleteCreatesMissingPRWithoutSessions(t *testing.T) {
	ws, origin := setupSliceWorkspace(t)
	seedOriginBranch(t, origin, allCompleteSeed)
	stubSeams(t)

	summaryCWDs := 0
	claudeRun = func(_ gocontext.Context, o claude.Opts) (*claude.Result, error) {
		if strings.Contains(o.Prompt, "Build only this slice") {
			t.Errorf("build session ran for an already-complete build:\n%s", o.Prompt)
			return &claude.Result{}, nil
		}
		// Summarize session: its CWD must be a worktree carrying the fetched
		// branch's work, not a fresh branch off the default branch.
		for _, f := range []string{"slice-1.txt", "slice-2.txt"} {
			if _, err := os.Stat(filepath.Join(o.CWD, f)); err != nil {
				t.Errorf("summarize CWD lacks %s — PR worktree not from the fetched branch: %v", f, err)
			}
		}
		summaryCWDs++
		return &claude.Result{ResultText: "TITLE: t\n\nDESCRIPTION:\nd"}, nil
	}
	checksRun = func(string) (string, error) { return "", nil }
	prURLForBranch = func(string, string) (string, error) { return "", nil }

	var prRepos [][]string
	prCreateAndLinkAll = func(_, _, _, _, _, _ string, repos []string, _ pr.FullNameFunc, _ string, _ *pr.CreateOpts) ([]string, error) {
		prRepos = append(prRepos, repos)
		return []string{"https://github.com/acme/api/pull/7"}, nil
	}

	res, err := Run(gocontext.Background(), liveSliceOpts(ws))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	ar := res.AgentResults[0]
	if ar.Error != nil {
		t.Fatalf("agent error: %v", ar.Error)
	}
	if len(ar.PRURLs) != 1 || ar.PRURLs[0] != "https://github.com/acme/api/pull/7" {
		t.Errorf("PR URLs = %v; want the created PR", ar.PRURLs)
	}

	if len(prRepos) != 1 {
		t.Fatalf("PR creation called %d times; want 1", len(prRepos))
	}
	if len(prRepos[0]) != 1 || prRepos[0][0] != "api" {
		t.Errorf("PR creation repos = %v; want [api]", prRepos[0])
	}
	if summaryCWDs != 1 {
		t.Errorf("summarize sessions = %d; want 1", summaryCWDs)
	}

	originLog := liveGit(t, origin, "log", liveBranch, "--format=%s")
	for _, marker := range []string{"minion:slice 1/2", "minion:slice 2/2"} {
		if got := strings.Count(originLog, marker); got != 1 {
			t.Errorf("origin has %d %q markers after ensure-PR re-run; want 1:\n%s", got, marker, originLog)
		}
	}
}

// TestRun_SliceLoop_AllCompleteWithExistingPRIsNoOp covers the idempotent
// re-run of a finished build: all markers present, PR already open. Nothing
// runs — no session of any kind, no PR creation — and the run exits success
// reporting the existing PR.
func TestRun_SliceLoop_AllCompleteWithExistingPRIsNoOp(t *testing.T) {
	ws, origin := setupSliceWorkspace(t)
	seedOriginBranch(t, origin, allCompleteSeed)
	stubSeams(t)

	claudeRun = func(_ gocontext.Context, o claude.Opts) (*claude.Result, error) {
		t.Errorf("claude session ran on a finished build:\n%s", o.Prompt)
		return &claude.Result{}, nil
	}
	checksRun = func(string) (string, error) { return "", nil }
	prURLForBranch = func(_, branch string) (string, error) {
		if branch != liveBranch {
			t.Errorf("PR lookup for branch %q; want %q", branch, liveBranch)
		}
		return "https://github.com/acme/api/pull/9", nil
	}
	prCreateAndLinkAll = func(string, string, string, string, string, string, []string, pr.FullNameFunc, string, *pr.CreateOpts) ([]string, error) {
		t.Error("PR creation called although the PR already exists")
		return nil, nil
	}

	res, err := Run(gocontext.Background(), liveSliceOpts(ws))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	ar := res.AgentResults[0]
	if ar.Error != nil {
		t.Fatalf("agent error: %v", ar.Error)
	}
	if len(ar.PRURLs) != 1 || ar.PRURLs[0] != "https://github.com/acme/api/pull/9" {
		t.Errorf("PR URLs = %v; want the existing PR's URL", ar.PRURLs)
	}
}

// TestRun_SliceLoop_ExcessMarkersAbortLoudly covers plan/branch disagreement:
// origin's branch carries more slice markers than the plan has slices. The
// run must fail before doing anything — no session, no PR, no guess about
// which plan is right.
func TestRun_SliceLoop_ExcessMarkersAbortLoudly(t *testing.T) {
	ws, origin := setupSliceWorkspace(t)
	seedOriginBranch(t, origin, []string{"minion:slice 1/3", "minion:slice 2/3", "minion:slice 3/3"})
	stubSeams(t)

	claudeRun = func(_ gocontext.Context, o claude.Opts) (*claude.Result, error) {
		t.Errorf("claude session ran despite plan/branch disagreement:\n%s", o.Prompt)
		return &claude.Result{}, nil
	}
	checksRun = func(string) (string, error) { return "", nil }
	prURLForBranch = func(string, string) (string, error) { return "", nil }
	prCreateAndLinkAll = func(string, string, string, string, string, string, []string, pr.FullNameFunc, string, *pr.CreateOpts) ([]string, error) {
		t.Error("PR creation called despite plan/branch disagreement")
		return nil, nil
	}

	res, err := Run(gocontext.Background(), liveSliceOpts(ws))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	ar := res.AgentResults[0]
	if ar.Error == nil {
		t.Fatal("agent result error = nil; want loud failure for markers exceeding the plan")
	}
	if !strings.Contains(ar.Error.Error(), "disagree") {
		t.Errorf("error %q does not name the plan/branch disagreement", ar.Error)
	}
}

// TestRun_SliceLoop_RetryThenPass covers the fix-it path: slice 1's checks
// fail once, one retry session scoped to the failure output runs, checks pass
// on re-run, and the loop advances to slice 2 and the PR as normal.
func TestRun_SliceLoop_RetryThenPass(t *testing.T) {
	ws, origin := setupSliceWorkspace(t)
	stubSeams(t)

	var prompts []string
	claudeRun = func(_ gocontext.Context, o claude.Opts) (*claude.Result, error) {
		prompts = append(prompts, o.Prompt)
		if !strings.Contains(o.Prompt, "Build only this slice") {
			return &claude.Result{ResultText: "TITLE: t\n\nDESCRIPTION:\nd"}, nil
		}
		return &claude.Result{}, nil
	}

	// Slice 1: fail, then pass on the post-retry re-run. Slice 2: pass.
	checkOutcomes := []error{fmt.Errorf("lint exploded"), nil, nil}
	checkCalls := 0
	checksRun = func(string) (string, error) {
		i := checkCalls
		checkCalls++
		if i < len(checkOutcomes) && checkOutcomes[i] != nil {
			return "boom output", checkOutcomes[i]
		}
		return "", nil
	}

	prCalled := 0
	prCreateAndLinkAll = func(string, string, string, string, string, string, []string, pr.FullNameFunc, string, *pr.CreateOpts) ([]string, error) {
		prCalled++
		return []string{"https://github.com/acme/api/pull/7"}, nil
	}

	res, err := Run(gocontext.Background(), liveSliceOpts(ws))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ar := res.AgentResults[0]; ar.Error != nil {
		t.Fatalf("agent error: %v", ar.Error)
	}

	// Session order: slice 1 build, fix-it retry, slice 2 build, summarize.
	if len(prompts) != 4 {
		t.Fatalf("want 4 sessions (build, fix-it, build, summarize), got %d", len(prompts))
	}
	if !strings.Contains(prompts[0], "Build only this slice: Slice 1") {
		t.Errorf("session 1 is not slice 1's build session")
	}
	if !strings.Contains(prompts[1], "boom output") || !strings.Contains(prompts[1], "lint exploded") {
		t.Errorf("fix-it session prompt not scoped to the failure output:\n%s", prompts[1])
	}
	if strings.Contains(prompts[1], "Build only this slice") {
		t.Errorf("fix-it session got a build prompt, not a fix-it prompt")
	}
	if !strings.Contains(prompts[2], "Build only this slice: Slice 2") {
		t.Errorf("session 3 is not slice 2's build session")
	}

	if checkCalls != 3 {
		t.Errorf("check runs = %d; want 3 (fail, re-run pass, slice 2 pass)", checkCalls)
	}
	originLog := liveGit(t, origin, "log", liveBranch, "--format=%s")
	for _, marker := range []string{"minion:slice 1/2", "minion:slice 2/2"} {
		if !strings.Contains(originLog, marker) {
			t.Errorf("origin branch missing marker %q after retry-then-pass:\n%s", marker, originLog)
		}
	}
	if prCalled != 1 {
		t.Errorf("PR creation called %d times; want 1", prCalled)
	}
}

// TestRun_SliceLoop_RetryThenFail_StopsChainWithEarlierSlicesPushed covers the
// abort path: slice 2 still fails after its retry, the run ends with an
// error, slice 1's commits and marker stay pushed on the branch, and no PR is
// created — "PR exists" keeps meaning "the build finished".
func TestRun_SliceLoop_RetryThenFail_StopsChainWithEarlierSlicesPushed(t *testing.T) {
	ws, origin := setupSliceWorkspace(t)
	stubSeams(t)

	var prompts []string
	claudeRun = func(_ gocontext.Context, o claude.Opts) (*claude.Result, error) {
		prompts = append(prompts, o.Prompt)
		return &claude.Result{}, nil
	}

	// Slice 1: pass. Slice 2: fail, and fail again after the retry.
	checkOutcomes := []error{nil, fmt.Errorf("tests exploded"), fmt.Errorf("tests still exploding")}
	checkCalls := 0
	checksRun = func(string) (string, error) {
		i := checkCalls
		checkCalls++
		if i < len(checkOutcomes) && checkOutcomes[i] != nil {
			return "test failure output", checkOutcomes[i]
		}
		return "", nil
	}

	prCalled := 0
	prCreateAndLinkAll = func(string, string, string, string, string, string, []string, pr.FullNameFunc, string, *pr.CreateOpts) ([]string, error) {
		prCalled++
		return []string{"https://github.com/acme/api/pull/7"}, nil
	}

	res, err := Run(gocontext.Background(), liveSliceOpts(ws))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	ar := res.AgentResults[0]
	if ar.Error == nil {
		t.Fatal("agent result error = nil; want failure for slice 2 exhausting its retry")
	}
	if !strings.Contains(ar.Error.Error(), "slice 2/2") {
		t.Errorf("error %q does not name the failed slice", ar.Error)
	}

	// Sessions: slice 1 build, slice 2 build, slice 2 fix-it. No summarize.
	if len(prompts) != 3 {
		t.Fatalf("want 3 sessions (build, build, fix-it), got %d", len(prompts))
	}
	if !strings.Contains(prompts[2], "test failure output") {
		t.Errorf("fix-it session prompt not scoped to slice 2's failure output")
	}

	// Slice 1 remains pushed; slice 2 never completed.
	originLog := liveGit(t, origin, "log", liveBranch, "--format=%s")
	if !strings.Contains(originLog, "minion:slice 1/2") {
		t.Errorf("completed slice 1 not pushed on origin:\n%s", originLog)
	}
	if strings.Contains(originLog, "minion:slice 2/2") {
		t.Errorf("failed slice 2 has a completion marker on origin:\n%s", originLog)
	}
	if prCalled != 0 {
		t.Errorf("PR creation called %d times on a failed run; want 0", prCalled)
	}
}

// TestRun_SliceLoop_RetryThenFail_PostsFailureComment covers the failure
// comment: when slice 2 still fails after its retry, exactly one comment
// lands on the triggering issue naming the failed slice (number, title,
// total), the branch holding the completed slices, and the resume
// instruction.
func TestRun_SliceLoop_RetryThenFail_PostsFailureComment(t *testing.T) {
	ws, _ := setupSliceWorkspace(t)
	stubSeams(t)

	claudeRun = func(_ gocontext.Context, _ claude.Opts) (*claude.Result, error) {
		return &claude.Result{}, nil
	}
	// Slice 1: pass. Slice 2: fail, and fail again after the retry.
	checkOutcomes := []error{nil, fmt.Errorf("tests exploded"), fmt.Errorf("tests still exploding")}
	checkCalls := 0
	checksRun = func(string) (string, error) {
		i := checkCalls
		checkCalls++
		if i < len(checkOutcomes) && checkOutcomes[i] != nil {
			return "test failure output", checkOutcomes[i]
		}
		return "", nil
	}
	prCreateAndLinkAll = func(string, string, string, string, string, string, []string, pr.FullNameFunc, string, *pr.CreateOpts) ([]string, error) {
		return nil, nil
	}

	type post struct{ repo, number, body string }
	var posts []post
	postIssueComment = func(repo, number, body string) error {
		posts = append(posts, post{repo: repo, number: number, body: body})
		return nil
	}

	res, err := Run(gocontext.Background(), liveSliceOpts(ws))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if ar := res.AgentResults[0]; ar.Error == nil {
		t.Fatal("agent result error = nil; want failure for slice 2 exhausting its retry")
	}

	if len(posts) != 1 {
		t.Fatalf("issue comments posted = %d; want exactly 1", len(posts))
	}
	p := posts[0]
	if p.repo != "acme/api" {
		t.Errorf("comment posted to repo %q; want acme/api", p.repo)
	}
	if p.number != "42" {
		t.Errorf("comment posted to issue %q; want 42", p.number)
	}
	for _, want := range []string{"slice 2/2", "Dry-run expansion", liveBranch, "resume"} {
		if !strings.Contains(p.body, want) {
			t.Errorf("comment body missing %q:\n%s", want, p.body)
		}
	}
}

// TestRun_SliceLoop_CommentPostingFailureDoesNotMaskBuildFailure covers the
// no-masking rule: when posting the failure comment itself fails, the run
// still reports the slice failure as its cause — never the posting error.
func TestRun_SliceLoop_CommentPostingFailureDoesNotMaskBuildFailure(t *testing.T) {
	ws, _ := setupSliceWorkspace(t)
	stubSeams(t)

	claudeRun = func(_ gocontext.Context, _ claude.Opts) (*claude.Result, error) {
		return &claude.Result{}, nil
	}
	checksRun = func(string) (string, error) { return "boom", fmt.Errorf("tests exploded") }
	prCreateAndLinkAll = func(string, string, string, string, string, string, []string, pr.FullNameFunc, string, *pr.CreateOpts) ([]string, error) {
		t.Error("PR created on a failed run")
		return nil, nil
	}
	postIssueComment = func(string, string, string) error {
		return fmt.Errorf("gh melted down")
	}

	res, err := Run(gocontext.Background(), liveSliceOpts(ws))
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	ar := res.AgentResults[0]
	if ar.Error == nil {
		t.Fatal("agent result error = nil; want the slice failure to survive the posting failure")
	}
	if !strings.Contains(ar.Error.Error(), "slice 1/2: checks failed after retry") {
		t.Errorf("error %q does not report the slice failure as the cause", ar.Error)
	}
	if strings.Contains(ar.Error.Error(), "gh melted down") {
		t.Errorf("posting error leaked into the reported cause: %q", ar.Error)
	}
}

// TestRun_PostsNoIssueCommentOutsideSliceFailure pins "the failure comment is
// the only comment the runtime ever posts": healthy sliced runs, dry-runs,
// and plan-less runs — even a plan-less run whose checks fail — post nothing.
func TestRun_PostsNoIssueCommentOutsideSliceFailure(t *testing.T) {
	countPosts := func(t *testing.T) *int {
		t.Helper()
		posts := 0
		postIssueComment = func(_, _, body string) error {
			posts++
			t.Errorf("issue comment posted:\n%s", body)
			return nil
		}
		return &posts
	}

	t.Run("healthy sliced run", func(t *testing.T) {
		ws, _ := setupSliceWorkspace(t)
		stubSeams(t)
		posts := countPosts(t)
		claudeRun = func(_ gocontext.Context, o claude.Opts) (*claude.Result, error) {
			if strings.Contains(o.Prompt, "Build only this slice") {
				if err := os.WriteFile(filepath.Join(o.CWD, "work.txt"), []byte(o.Prompt[:20]), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			return &claude.Result{ResultText: "TITLE: t\n\nDESCRIPTION:\nd"}, nil
		}
		checksRun = func(string) (string, error) { return "", nil }
		prCreateAndLinkAll = func(string, string, string, string, string, string, []string, pr.FullNameFunc, string, *pr.CreateOpts) ([]string, error) {
			return []string{"https://github.com/acme/api/pull/7"}, nil
		}

		res, err := Run(gocontext.Background(), liveSliceOpts(ws))
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if ar := res.AgentResults[0]; ar.Error != nil {
			t.Fatalf("agent error: %v", ar.Error)
		}
		if *posts != 0 {
			t.Errorf("healthy run posted %d issue comments; want 0", *posts)
		}
	})

	t.Run("dry-run", func(t *testing.T) {
		stubSeams(t)
		posts := countPosts(t)

		var runErr error
		captureStdout(t, func() {
			_, runErr = Run(gocontext.Background(), sliceTestOpts(slicedProgram(), []slices.Comment{
				{Author: "jcleira", Body: testPRDComment},
				{Author: "jcleira", Body: testPlanComment},
			}))
		})
		if runErr != nil {
			t.Fatalf("Run: %v", runErr)
		}
		if *posts != 0 {
			t.Errorf("dry-run posted %d issue comments; want 0", *posts)
		}
	})

	t.Run("plan-less run with failing checks", func(t *testing.T) {
		ws, _ := setupSliceWorkspace(t)
		stubSeams(t)
		posts := countPosts(t)
		claudeRun = func(_ gocontext.Context, o claude.Opts) (*claude.Result, error) {
			if err := os.WriteFile(filepath.Join(o.CWD, "work.txt"), []byte("w"), 0o644); err != nil {
				t.Fatal(err)
			}
			return &claude.Result{}, nil
		}
		checksRun = func(string) (string, error) { return "boom", fmt.Errorf("checks exploded") }
		prCreateAndLinkAll = func(string, string, string, string, string, string, []string, pr.FullNameFunc, string, *pr.CreateOpts) ([]string, error) {
			t.Error("PR created despite failing checks")
			return nil, nil
		}

		opts := liveSliceOpts(ws)
		opts.IssueComments = []slices.Comment{{Author: "jcleira", Body: testPRDComment}}
		res, err := Run(gocontext.Background(), opts)
		if err != nil {
			t.Fatalf("Run: %v", err)
		}
		if ar := res.AgentResults[0]; ar.Error == nil {
			t.Fatal("agent result error = nil; want failure for failing checks")
		}
		if *posts != 0 {
			t.Errorf("plan-less failing run posted %d issue comments; want 0 — the failure comment belongs to the slice path only", *posts)
		}
	})
}
