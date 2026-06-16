# 07 — Slice plan as a comment + manual one-PR build (replaces #4)

**Source PRD**: [../prd.md](../prd.md) (see the Design-revised banner)
**Replaces**: [04 — slicer agent + child issues + status comment](./04-slicer-child-issues.md)
**Blocked by**: [03 — prd-writer agent + PRD comment + parent label](./03-prd-writer-comment-label.md)

> **Status: shipped.** Records the design that replaced #4's child-issue
> + auto-cascade approach, after that approach proliferated issues and
> collided feature PRs on a shared branch.

## What to build

The research run stops at *review artifacts*. After the PRD comment
(slice #3), the `publisher` posts the slice plan as a **second comment**
on the parent issue and labels the parent `minion-research-completed`.
It opens **no** child issues, applies **no** `minion-approved`, and
triggers **no** implementation.

Implementation is a separate, manual, single-PR step. When jcleira has
reviewed the PRD + slice plan, he labels the **parent** `minion-approved`
(or comments `/minion build`); `minion.yml` fires `implement.md` once on
the parent and produces **one** feature PR.

For this to yield one clean PR per build, the minions runtime must give
each build its own branch. The runtime previously derived the branch
from `prog.ID + "-" + agent.Name` only — a constant
`minion/implement-implement` shared across every build — so sequential
builds collided into whatever PR was open on that branch. The runtime
now appends the issue number: `minion/<prog>-<agent>-<issue>`.

## Acceptance criteria

- [x] `research.md`'s `publisher` posts the slice plan as a single
  second comment on the parent, first line
  `<!-- minion:research-slices parent=#<N> -->`, slices rendered as
  readable Markdown.
- [x] `publisher` opens no child issues and applies no `minion-approved`
  / `minion-ready` label; the research run's only side effects are the
  PRD comment, the slice-plan comment, and the `minion-research-completed`
  label.
- [x] Parent issue is left open; the research run never adds `minion-done`.
- [x] Labeling the parent `minion-approved` (or `/minion build`) fires
  `implement.md` once on the parent and produces exactly one feature PR.
- [x] Each build uses a unique branch `minion/<prog>-<agent>-<issue>`
  (minions ≥ v0.0.6); two builds never share a branch/PR.
- [x] Verified end-to-end: partio-io/cli#437 → PRD + slice-plan comments,
  no child issues, then one PR (#443) on branch
  `minion/implement-implement-437`.

## Modules touched

- `partio-io/cli/.minions/programs/research.md` (`publisher` rewrite) —
  partio-io/cli#436.
- `partio-minions` runtime `internal/executor` + `cmd/minions/run.go`
  (unique per-issue taskID) — partio-io/minions#130, released v0.0.6.
- `partio-io/cli` workflow pins bumped to `minions@v0.0.6` —
  partio-io/cli#439.

## Test prior art

- `partio-cli/.minions/programs/propose.md` — `gh issue comment`
  patterns from inside an agent prompt.
- `internal/executor/executor_test.go` — `TestBuildTaskID` table test +
  the regression guard that two issues never share a taskID.

## Out of scope

- Idempotency / skip-if-marker-exists on the two comments — slice
  [#5](./05-idempotency-rerun-skip.md) (premise revised for the comment
  model).
- First production smoke against a real `minion-proposal` issue — slice
  [#6](./06-production-smoke-run.md).
