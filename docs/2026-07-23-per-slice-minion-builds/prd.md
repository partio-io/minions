# Per-Slice Minion Builds

## Problem Statement

When a researched issue is approved for building, the minion runtime
executes the entire issue in a single Claude session. The research
pipeline already decomposes complex issues into ordered vertical
slices and posts them as a slice-plan comment, but nothing consumes
that plan: the implement program receives the whole issue as one blob
and one agent builds all slices in one monotonically growing context.

Measured consequences (cli issue #28 → PR #553, run 29933513692):

- One agent, 28 turns, ~891k cache-read tokens — the working context
  grew with every file read, test run, and edit across all 4 slices.
- Peak context scales with Σ(all slices), not max(one slice); late
  slices are built while earlier slices' edits and test output clutter
  the window.
- One failure anywhere kills the whole issue's build; a re-run starts
  from zero and compounds artifacts (duplicate PRs) instead of
  resuming.
- Slice-mandated details get dropped silently — #553 shipped 2 of 4
  slices without their required tests and regressed an acceptance
  criterion, while CI stayed green.

## Solution

The build of a researched issue becomes slice-by-slice, invisible from
the outside: same trigger (`minion-approved` label or `/minion build`
comment on the parent issue), same single multi-commit PR at the end,
same done/failed labels.

Internally the runtime parses the slice-plan comment and runs one
fresh Claude session per slice, in plan order, on one shared branch.
Each slice session sees only what it needs — the issue, the plan, and
its own slice — plus the previous slices' work as code on the branch,
not as conversation history. Each slice is checked (`make test`,
`make lint`) and marked complete with a marker commit before the next
begins. If a slice fails, everything completed stays pushed; the issue
is labeled failed with a comment naming the failed slice, and
re-triggering the build resumes from that slice instead of starting
over. The PR opens only when the final slice passes.

Issues without a slice plan build exactly as today, in a single
session.

## User Stories

1. As the maintainer, I want each slice of a researched issue built in
   its own fresh Claude session, so that peak context stays bounded by
   the largest slice instead of the whole issue.
2. As the maintainer, I want slices built in the plan's dependency
   order on one shared branch, so that later slices build on earlier
   slices' code the way the plan intended.
3. As the maintainer, I want each slice session told to build only its
   own slice, so that agents stop skipping slice-level requirements
   (like per-slice tests) that get lost in a whole-issue blob.
4. As a PR reviewer, I want one multi-commit PR per issue — not one PR
   per slice — so that I review a feature once, with slice boundaries
   visible as commits.
5. As the maintainer, I want the branch name to stay stable per issue
   across all slice sessions, so that the existing done/failed
   workflow automation keeps working unchanged.
6. As the maintainer, I want `make test` and `make lint` run after
   every slice, so that a broken slice is caught and retried at the
   slice boundary instead of poisoning the slices after it.
7. As the maintainer, I want a slice's fix-it retry scoped to that
   slice, so that retries operate on a small context instead of the
   whole issue's history.
8. As the maintainer, I want each completed slice recorded with a
   marker commit and pushed immediately, so that completed work
   survives any later failure.
9. As the maintainer, I want a failed run to label the issue
   `minion-failed` and comment which slice failed on which branch, so
   that I know exactly where the build stopped without reading CI
   logs.
10. As the maintainer, I want re-triggering the build after a failure
    to resume from the first incomplete slice, so that a failure late
    in the plan doesn't cost me the slices already built.
11. As the maintainer, I want re-triggering the build after a success
    to detect that all slices are complete and simply ensure the PR
    exists, so that accidental re-runs are idempotent instead of
    compounding duplicate branches and PRs.
12. As the maintainer, I want the PR opened only when the final slice
    has passed its checks, so that an open PR still means "the build
    finished" for both me and the workflow automation.
13. As the maintainer, I want a malformed or unparseable slice plan to
    fail the run loudly with the parse error, so that a decomposed
    issue is never silently rebuilt as one big-context blob.
14. As the maintainer, I want issues without a slice-plan comment to
    build exactly as today in a single session, so that simple issues
    and existing flows keep working with zero migration.
15. As the maintainer, I want to hand-write a slice-plan comment on
    any issue using the same marker format, so that I can get sliced
    builds without running the research pipeline first.
16. As the maintainer, I want only programs that opt in to slice-aware
    building to use it, so that other programs (doc updates,
    proposals) are unaffected by the new machinery.
17. As the maintainer, I want each slice session to receive the issue
    body, the PRD, the full slice plan, and its own slice marked as
    the target, so that the agent has design context without carrying
    other slices' build history.
18. As the maintainer, I want no per-slice progress comments on the
    issue, so that the issue thread stays readable; the final PR and
    any failure comment are the record.
19. As the operator of the minion runner, I want the resumed run to
    fetch the issue's minion branch from origin before deciding what
    is complete, so that resume decisions are made against pushed
    reality, not a stale local clone.
20. As the operator of the minion runner, I want the workflow timeout
    raised to accommodate multi-slice sequential sessions, so that
    legitimate 4-slice builds are not killed mid-run.
21. As the maintainer, I want the runtime change released and the
    workflow's version pin bumped as an explicit final step, so that
    the feature actually reaches production runs instead of silently
    staying on the old pinned version.

## Implementation Decisions

- **Language:** Go — the minions repo's existing language; no new
  service. Program and workflow changes in the cli repo are
  markdown/YAML config only.
- **New `slices` domain module** in the minions runtime, the deep
  module of this feature. Two responsibilities behind a small
  interface: (1) parse a slice-plan comment (the
  `minion:research-slices` marker followed by `Proposed slices`
  sections) into an ordered, validated plan — title, description,
  acceptance criteria, modules touched, out of scope per slice;
  (2) compute the resume point for a plan from the marker commits
  present on a branch. Validation is strict: missing marker, gaps in
  numbering, or empty acceptance criteria fail with a descriptive
  error. No fallback path.
- **Executor slice loop.** When the program opts in (frontmatter flag)
  and the issue carries a parseable slice plan, the executor expands
  the single implement agent into N sequential slice sessions. Each
  iteration: worktree at the shared branch tip → fresh Claude session
  → checks → scoped retry on failure → empty marker commit recording
  slice number and total → push. The task identifier (and therefore
  the branch name) stays the same across all slices — one branch, one
  PR per issue.
- **Per-slice prompt composition.** Issue title and body, the PRD
  comment, the full slice plan for orientation, and the target slice
  singled out with an explicit "build only this slice" directive.
  Previous slices are visible as code and commits on the branch, not
  as prompt history.
- **Resume semantics.** On every slice-aware run: fetch the issue's
  minion branch from origin; if it exists, count marker commits to
  determine completed slices; skip those and continue from the first
  incomplete slice. All slices complete → ensure the PR exists and
  exit success (idempotent re-run). Branch absent → start from slice
  one.
- **Failure semantics.** A slice that still fails checks after its
  retry ends the run: completed slices remain pushed, the issue gets
  the failed label plus a comment naming the failed slice and the
  branch. No PR is opened — "PR exists" remains the workflow's
  success signal.
- **Comment plumbing.** The run command already fetches issue
  comments; it additionally passes them to the executor structurally
  (not only as the rendered context blob) so slice-plan and PRD
  comments can be identified by their markers.
- **Worktree create-or-resume.** Worktree creation gains a mode that
  checks out the existing remote minion branch when resuming, instead
  of always branching fresh from the default branch.
- **Program opt-in.** The implement program declares slice-aware
  building via frontmatter; no other program opts in. Trigger,
  labels, and the done/failed workflow steps are unchanged.
- **Deploy chain.** Feature lands in the minions runtime first and is
  released; then the cli repo bumps the pinned runtime version in its
  workflows and raises the minion workflow timeout (30 → 60 minutes).
  The runtime change is inert in production until the pin bump.

## Testing Decisions

- Tests assert external behavior — parsed plans, chosen resume
  points, commits and pushes present on a branch, PR-creation
  decisions — never internal call sequences.
- **Every module is tested. Not negotiated.**
  - Slice-plan parsing: fixture comments — the publisher's exact
    format, a hand-written plan, and malformed variants (missing
    marker, numbering gap, empty criteria) that must fail with
    descriptive errors.
  - Resume-point computation: synthetic branch histories in temp git
    repos — no markers, partial markers, all markers, marker counts
    exceeding the plan.
  - Executor slice loop: temp-git-repo runs with stubbed session and
    check outcomes covering advance-on-success, retry-then-fail stops
    the chain, resume skips completed slices, and PR-only-after-final.
  - Worktree create-or-resume: existing-branch and fresh-branch paths.
- Deliberately untested: live Claude sessions and real GitHub calls.
  Covered instead by a dry-run against a fixture issue and one staged
  end-to-end run on a real test issue before the version pin bump.
- Prior art: the runtime's existing table-driven tests with temp git
  repos and env isolation, and the issue-context rendering regression
  tests around comment selection.

## Out of Scope

- Child GitHub issues per slice — the plan stays a comment on the
  parent issue.
- One PR per slice — single multi-commit PR per issue stands.
- Parallel slice execution — plans are dependency-ordered; execution
  is strictly sequential.
- Changes to the research publisher's slice-comment format — the
  parser targets the format already in production.
- Slice-aware building for other programs (doc updates, proposals,
  readme updates).
- Cross-repo slice plans — the docs-repo coverage gap is a separate
  issue.
- A general fix for stale workspace clones — this feature fetches the
  branch it resumes; repo-wide fetch behavior is a separate issue.
- Per-slice progress comments, notifications, or dashboards.

## Further Notes

- Evidence base: cli issue #28 built as PR #553 by run 29933513692 —
  one agent, 28 turns, ~891k cache-read tokens, 2 of 4 slices shipped
  without their mandated tests.
- The runtime already runs multiple agents sequentially, each in a
  fresh session and worktree, within one run (the research program's
  five-agent chain). The slice loop generalizes that same primitive
  from static program-declared agents to plan-driven dynamic ones.
- Accepted trade-off: each slice session re-orients on the repository
  cold, so total wall-clock rises; in exchange peak context drops from
  the sum of all slices to the largest single slice, and failures
  become resumable at slice granularity.
- Hand-written slice-plan comments (same marker format) give sliced
  builds on any issue without a research run — free consequence of
  parsing the comment rather than research-run state.
- Resume counts marker commits on the pushed branch, which also
  retires the known "re-runs compound duplicate PRs" trap for
  slice-aware builds.
