# 05 — Idempotency: re-runs skip existing artifacts

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [04 — slicer agent + child issues + status comment](./04-slicer-child-issues.md)

> **Premise revised — see slice [#07](./07-slice-plan-comment-manual-build.md).**
> Written when a run produced three artifact types (PRD comment, child
> issues, status comment). Under the shipped design a run produces only
> **two comments** on the parent — the PRD comment (`minion:research
> run-id=`) and the slice-plan comment (`minion:research-slices
> parent=#N`) — and no child issues. The skip-if-marker-exists work still
> applies, but only to those two comments; the child-issue and
> status-comment markers below are obsolete. Not yet built.

## What to build

Make `research.md` safe to re-run against the same parent issue
without producing duplicate artifacts. As of slice
[#4](./04-slicer-child-issues.md), the publisher writes three kinds of
GitHub artifacts, each tagged with a marker:

- PRD comment on parent — first line
  `<!-- minion:research run-id=<sha7> -->`
- Child issue body — first line
  `<!-- minion:slice parent=#<N> slice=<n>/<total> -->`
- Status comment on parent — first line
  `<!-- minion:research-status parent=#<N> -->`

This slice extends the `publisher` agent's prompt so that, before
each write, it queries existing GitHub state and skips the write if
a matching marker already exists. Concretely:

- Before posting the PRD comment, query
  `gh issue view <N> --repo <repo> --json comments` and inspect each
  comment body. If any comment's first line begins with
  `<!-- minion:research run-id=` (any run-id), skip the write or
  regenerate the comment in place via `gh issue comment edit` (the
  prompt may pick either; the contract is "no duplicate"). Per the
  PRD's *Idempotency contract* section: "regenerating instead" is
  acceptable.
- Before opening child issues, query
  `gh issue list --repo <repo> --search 'in:body
  <!-- minion:slice parent=#<N>'`. If matches exist, skip creating
  child issues whose `slice=<n>/<total>` marker already appears, or
  regenerate them in place.
- Before posting the status comment, apply the same
  query-and-skip-or-replace logic against the parent's existing
  comments using the `minion:research-status parent=#<N>` marker.

The intent is operational: a developer who re-labels a parent issue
(or re-runs the workflow from the GitHub UI) gets a clean update
rather than duplicates. The exact strategy (skip vs replace) is a
publisher-prompt decision; what must hold is the no-duplicate
invariant.

## Acceptance criteria

- [ ] `publisher` agent prompt instructs the agent to query existing
  parent-issue comments via `gh issue view --json comments` before
  posting the PRD comment, and to skip or replace if a comment with
  the `minion:research run-id=` marker prefix already exists.
- [ ] `publisher` agent prompt instructs the agent to query existing
  child issues via `gh issue list --search` (or equivalent) using the
  `minion:slice parent=#<N>` marker, and to skip or replace child
  issues whose `slice=<n>/<total>` marker already exists for the
  current parent.
- [ ] `publisher` agent prompt instructs the agent to query existing
  parent-issue comments for the
  `minion:research-status parent=#<N>` marker before posting the
  status comment, and to skip or replace if it already exists.
- [ ] Re-run smoke: trigger the workflow twice on the same test
  parent issue (e.g., remove and re-apply the `minion-research`
  label, or trigger via comment after a previous run finished).
  After the second run:
  - Exactly one PRD comment exists on the parent (count by marker
    prefix).
  - The set of child issues for the parent has not grown (no
    duplicates by `slice=<n>/<total>` marker).
  - Exactly one status comment exists on the parent (count by
    marker).
- [ ] No regressions: a first run on a fresh test parent issue still
  produces the full set of artifacts (PRD comment + N child issues +
  status comment) — slice [#4](./04-slicer-child-issues.md) behavior
  is preserved.

## Modules touched

- `partio-io/cli/.minions/programs/research.md` (extend
  `publisher` prompt only)

No changes to `research.yml`. No changes to `partio-minions` Go
runtime.

## Test prior art

- `partio-cli/.minions/programs/propose.md` step 4 ("Check if a
  proposal already exists: `gh issue list --repo <this-repo> --label
  minion-proposal --search "<feature-id>" --limit 1`") — the same
  pattern of query-existing-before-write, applied to
  `minion-proposal` issues. Same `gh` flags and same idea, narrower
  marker convention.
- `partio-minions/internal/program/parse_test.go` — re-validates the
  program file after the prompt edits.

## Out of scope

- Diff/version tracking for the PRD comment. PRD: *Out of Scope* —
  first version overwrites or regenerates; no history is kept.
- An integration test that runs `research.md` twice and asserts no
  duplicates programmatically. PRD's *Testing Decisions* note:
  "Build only if a real re-run produces duplicates." For now, smoke
  is sufficient.
- Cleaning up child issues created in earlier (now-superseded)
  research runs. The publisher reconciles, it does not garbage
  collect.
