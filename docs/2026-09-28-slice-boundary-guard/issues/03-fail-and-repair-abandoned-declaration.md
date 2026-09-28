# 03 — Fail and repair an abandoned declaration

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [01 — Prefactor: route both check runs through one verification step](./01-prefactor-one-verification-step.md), [02 — List earlier slice contributions in each slice prompt](./02-list-earlier-contributions-in-prompt.md)

## What to build

The headline behavior. When a slice rebuilds what an earlier slice
already built, the run notices, the slice repairs itself, and the build
continues.

After a slice's session finishes, and before that slice's work is
committed, the guard reads the working tree. When an earlier slice
contributed a declaration and nothing in the tree references it any
more, the check fails.

A failed check already has a defined path, reshaped in issue 01: the
executor runs exactly one fix session whose prompt is the failure text,
then verifies again. The guard reuses that path and adds no new failure
route, no new retry budget and no new configuration key.

The guard's failure text is what makes the repair land. It names the
abandoned identifier, its file, the slice that added it, and the
required repair — call the earlier code and drop the duplicate. Nothing
else is threaded into the fix session; the text carries the context.

The repair direction is fixed. The current slice reuses the earlier
slice's work. It does not delete it. Deletion is the outcome the
downstream dead-code audit already produces, and it is the outcome this
whole feature exists to prevent.

When the fix session cannot repair the tree, the slice fails and the run
stops at that slice, exactly as any failed check does today. The earlier
slices stay pushed and the issue gets the usual resume comment.

A successful repair is silent. No pull request line, no issue comment.

Two details matter for correctness:

- The check runs before the current slice's work is committed, so
  references are judged against the files on disk, not against commits.
  That is what lets the guard see a duplicate the session has written
  but not yet committed.
- A declaration that only its own test calls counts as abandoned. That
  is the exact shape the failure on issue #31 left behind.

## User stories covered

PRD stories 2, 3, 4, 5, 6, 7, 8, 10, 13, 14, 15, 16, 17, 19, 25, 27.

## Acceptance criteria

- [ ] `internal/sliceguard` reports a finding when an earlier slice's
      contribution has no remaining reference in the working tree.
- [ ] References are judged against the files on disk, so a duplicate
      written but not yet committed is seen.
- [ ] A declaration referenced only by its own test counts as abandoned.
- [ ] The guard runs through the verification value from issue 01, so a
      finding fails the slice's checks on the existing path.
- [ ] The failure text names the identifier, its file, the slice that
      added it, and states that the repair is to call the earlier code
      and drop the duplicate.
- [ ] A finding triggers exactly one fix session, then one
      re-verification, matching the existing retry contract.
- [ ] When the fix session repairs the tree, the run continues and
      reaches pull request creation.
- [ ] When the fix session does not repair the tree, the slice fails,
      the run stops there, and the earlier slices stay pushed.
- [ ] A successful repair adds no pull request line and no issue
      comment.
- [ ] The guard runs for every worktree in a multi-repo build.
- [ ] A repository whose language the guard cannot analyze yields no
      findings and never fails a slice.
- [ ] Slice one is never failed by the guard, and a run that trips
      nothing behaves exactly as it does today.

## Modules touched

- `internal/sliceguard` — the reference scan and the finding report.
- `internal/executor` — composing the guard into the verification value.

## Test prior art

The live slice-loop tests are the primary seam: they build a real
repository with an origin and drive the whole loop, and they already
contain retry-then-pass and retry-then-fail cases that are the closest
analogue to this slice's two headline behaviors. Add the repair-continue
and repair-fails cases beside them. The detection matrix itself belongs
at the `sliceguard` seam, using the temporary-repository helper pattern
from the git package's tests.

## Out of scope

- Exempting a contribution that a later slice will legitimately consume.
  Issue 04 adds that; until it lands, a plan that defers consumption by
  more than one slice can trip the guard.
- Deleted files. Issue 05.
- Code that is still called but inert. That stays with the cli dead-code
  audit and is out of scope for the whole PRD.
