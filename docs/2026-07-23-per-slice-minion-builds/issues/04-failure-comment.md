# 04 — Failure comment

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [02 — Live slice loop](./02-live-slice-loop.md)

## What to build

When a slice still fails its checks after the retry, the runtime
posts one comment on the issue before exiting: which slice failed
(number and title, out of how many), which branch holds the completed
slices, and that re-triggering the build resumes from the failed
slice. This is the only comment the runtime ever posts on an issue —
healthy runs stay silent, and the workflow's existing generic
failed-label step remains as the backstop it is today.

The comment opens with a stable machine marker (the same convention
as the research pipeline's comments) so future tooling can identify
it, and it is written for the human reading the issue: no log dumps,
just where the build stopped and what to do next.

## User stories covered

9 — and it completes the failure half of the PRD's failure semantics
(the work-stays-pushed half landed in issue 02).

## Acceptance criteria

- [x] A slice failing after retry posts exactly one issue comment
      naming the failed slice (number, title, total), the branch, and
      the resume instruction.
- [x] The comment starts with a stable machine marker following the
      existing minion comment-marker convention.
- [x] Healthy runs, plan-less runs, and dry-runs post nothing.
- [x] A failure in comment posting itself does not mask the build
      failure — the run still exits non-zero with the slice failure
      as the reported cause.
- [x] Tests cover comment content composition and the
      posting-failure-does-not-mask path; posting is stubbed, not
      live.

## Modules touched

- `executor` — failure path wiring
- A small comment-posting helper alongside the existing PR-creation
  plumbing (same credentials and CLI surface PR creation already
  uses)

## Test prior art

- Composition + stubbed-side-effect tests in the executor package.
- The run command's comment-rendering tests for tone/format of
  machine-marked bodies.

## Out of scope

- Per-slice progress comments on healthy runs — permanently out, per
  PRD.
- Changing the workflow's generic failed-label/comment step — it
  stays as backstop; issue 05 touches the workflow only for timeout
  and pin.
