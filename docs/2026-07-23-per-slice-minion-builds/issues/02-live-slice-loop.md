# 02 — Live slice loop

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [01 — Dry-run walking skeleton](./01-dry-run-walking-skeleton.md)

## What to build

A live (non-dry-run) slice-aware run builds the issue slice by slice:
for each slice in plan order, the executor creates a worktree at the
shared branch's current tip, runs a fresh Claude session with that
slice's prompt (composed in issue 01), runs the repo checks, retries
once with a failure-scoped fix-it session if they fail, then records
completion with an empty marker commit carrying the slice number and
total, and pushes the branch. The next slice starts from the pushed
tip, seeing earlier slices as code and commits — never as conversation
history.

The task identifier, and therefore the branch name, stays constant
across all slices — one branch, one PR per issue, so the existing
done/failed workflow automation is untouched. The PR is created only
after the final slice passes its checks. A slice that still fails
after its retry ends the run with an error: completed slices remain
pushed on the branch, and no PR is opened — "PR exists" keeps meaning
"the build finished". No per-slice comments are posted anywhere.

## User stories covered

1, 2, 4, 5, 6, 7, 8, 12, 18.

## Acceptance criteria

- [x] Each slice runs as its own fresh Claude session in a worktree
      created at the branch's current tip; slice N+1 sees slice N's
      commits.
- [x] Checks run after every slice; a failure triggers one
      fix-it retry session scoped to that slice's failure output.
- [x] After checks pass, the executor creates an empty marker commit
      identifying the slice (number and total) and pushes the branch.
- [x] The branch name is identical across all slices and matches the
      single-session path's naming, so downstream workflow lookups by
      head branch keep working.
- [x] The PR is created once, after the final slice passes, and
      carries all slices' commits.
- [x] A slice failing after retry aborts the run with a non-zero
      result; completed slices stay pushed; no PR exists.
- [x] No issue comments are posted by the runtime during a healthy
      run.
- [x] Executor tests with stubbed session/check outcomes cover:
      advance-on-success, retry-then-pass, retry-then-fail stops the
      chain with earlier slices pushed, and PR-only-after-final.

## Modules touched

- `executor` — the slice loop itself
- `worktree` — creating a worktree at the current branch tip between
  slices (full create-or-resume from origin lands in issue 03)
- `git` — empty marker commit + push helpers

## Test prior art

- Executor tests with temp git repos and stubbed outcomes in the
  executor package.
- Worktree create/cleanup tests in the worktree package.
- Table-driven git helper tests using `t.TempDir()` repos in the git
  package.

## Out of scope

- Resuming from a previously pushed branch, marker counting on run
  start, and idempotent re-runs — issue 03 (this issue always starts
  from slice one on a fresh branch).
- The failure comment naming the failed slice — issue 04 (this issue
  fails with today's generic error surface).
- cli repo config, release, or pin bump — issue 05.
