# 03 — Resume & idempotent re-run

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [02 — Live slice loop](./02-live-slice-loop.md)

## What to build

Re-triggering a slice-aware build no longer starts from zero. At the
start of every slice-aware run, the runtime fetches the issue's minion
branch from origin. If the branch exists, the `slices` module counts
the marker commits on it to compute the resume point: completed
slices are skipped and the run continues at the first incomplete
slice, in a worktree checked out from the fetched branch instead of a
fresh branch off the default branch.

If every slice is already marked complete, the run ensures the PR
exists (creating it if the previous run died between final push and
PR creation) and exits successfully — re-running a finished build is
a no-op, not a duplicate. If the branch does not exist on origin, the
run starts from slice one exactly as issue 02 built it. Resume
decisions are made only against the fetched remote state, never
against whatever a stale local clone happens to contain.

## User stories covered

10, 11, 19.

## Acceptance criteria

- [x] The `slices` module computes a resume point from a branch's
      marker commits: none → start at slice one; K of N → start at
      K+1; all N → nothing to build.
- [x] A slice-aware run fetches the issue's minion branch from origin
      before deciding where to start; local-only state is never
      trusted.
- [x] Resuming checks out a worktree from the fetched branch and
      continues the loop; completed slices run no session at all.
- [x] A re-run with all slices complete ensures the PR exists and
      exits success — no new branch, no duplicate PR, no session.
- [x] Marker counts exceeding the plan length fail loudly (plan and
      branch disagree) rather than guessing.
- [x] Tests cover resume-point computation against synthetic branch
      histories (no/partial/all/excess markers) and the executor's
      skip-completed + all-done-ensures-PR paths.

## Modules touched

- `slices` — resume-point computation (second half of the module)
- `worktree` — create-or-resume: worktree from an existing remote
  branch
- `executor` — resume wiring at loop start
- `git` — fetch-branch and marker-listing helpers

## Test prior art

- Temp-git-repo tests with synthetic commit histories in the git and
  worktree packages.
- Executor path tests with stubbed outcomes in the executor package.

## Out of scope

- The failure comment that tells the human which slice to resume
  from — issue 04.
- Any general fix for stale workspace clones beyond fetching the one
  branch this run resumes (tracked separately, out of scope per PRD).
