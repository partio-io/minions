# 05 — Idempotency: re-runs skip existing comments

**Blocked by**: None (the publish flow is shipped)

> Not yet built.

## What to build

Make `research.md` safe to re-run against the same parent issue without
producing duplicate comments. The `publisher` posts two marker-tagged
comments on the parent:

- PRD comment — first line `<!-- minion:research run-id=<sha7> -->`
- Slice-plan comment — first line `<!-- minion:research-slices parent=#<N> -->`

Extend the `publisher` prompt so that, before each write, it queries the
parent's existing comments and skips (or regenerates in place) if a
matching marker already exists. The invariant is "no duplicate"; skip vs.
replace is a publisher-prompt decision.

## Acceptance criteria

- [ ] Before posting the PRD comment, `publisher` queries existing parent
  comments (`gh issue view <N> --json comments`) and skips or replaces if
  a `minion:research run-id=` marker already exists.
- [ ] Before posting the slice-plan comment, `publisher` does the same
  against the `minion:research-slices parent=#<N>` marker.
- [ ] Re-run smoke: trigger the workflow twice on the same test parent.
  After the second run, exactly one PRD comment and one slice-plan
  comment exist (counted by marker).
- [ ] No regression: a first run on a fresh parent still posts both
  comments.

## Modules touched

- `partio-io/cli/.minions/programs/research.md` (extend `publisher`
  prompt only).

## Test prior art

- `partio-cli/.minions/programs/propose.md` — the
  query-existing-before-write pattern (`gh issue list … --search`),
  applied to `minion-proposal` issues.

## Out of scope

- Diff/version tracking for the comments — the first version overwrites
  or regenerates; no history is kept.
- A programmatic integration test (build only if a real re-run produces
  duplicates).
