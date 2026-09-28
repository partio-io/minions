# 05 — Catch a slice that deletes an earlier slice file

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [03 — Fail and repair an abandoned declaration](./03-fail-and-repair-abandoned-declaration.md)

## What to build

The other half of the rule the `implement` program already states:
earlier slices are committed on your branch, so build on their work and
never redo or undo it.

Issue 03 covers "redo" — a slice rebuilds earlier work and leaves it
unreferenced. This slice covers "undo" — a slice removes a file that an
earlier slice added.

The guard reports a finding when the current slice's working tree no
longer contains a file that an earlier slice in this run added. The
finding flows through the same verification value, the same one-shot fix
session and the same failure path as issue 03. The repair direction is
the same: restore the earlier slice's file and reuse it.

The commit partition from issue 02 already knows which files each slice
added, so this costs little beyond the reporting itself.

## User stories covered

PRD story 9.

## Acceptance criteria

- [ ] The guard reports a finding when the working tree no longer
      contains a file an earlier slice in this run added.
- [ ] The finding names the deleted file and the slice that added it,
      and states that the repair is to restore and reuse it.
- [ ] The finding fails the slice's checks through the same verification
      value as an abandoned declaration.
- [ ] A repaired deletion lets the run continue, with no pull request
      line and no issue comment.
- [ ] A deletion the fix session does not repair fails the slice and
      stops the run, with earlier slices still pushed.
- [ ] A file the base branch carried, deleted by a slice, is not
      reported; only files this run's earlier slices added.
- [ ] A file an earlier slice added and the current slice renamed is
      reported, since the earlier path no longer exists.

## Modules touched

- `internal/sliceguard` — deletion findings from the existing commit
  partition.

## Test prior art

Add the deletion cases at the `sliceguard` seam beside the abandonment
cases from issue 03, using the temporary-repository helper pattern from
the git package's tests. One end-to-end case belongs in the live
slice-loop tests, beside the repair cases added in issue 03.

## Out of scope

- A slice that edits lines an earlier slice added. The PRD excludes it,
  because a slice must stay free to edit earlier code inside its own
  scope.
- Restoring the file automatically. The fix session does the repair.
