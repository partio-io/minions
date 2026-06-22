# 06 — First production smoke run

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [05 — Idempotency: re-runs skip existing artifacts](./05-idempotency-rerun-skip.md)

> **Procedure revised — see slice [#07](./07-slice-plan-comment-manual-build.md).**
> The shipped design has no auto-cascade to pause, so the "remove
> `minion-approved` from each child before it fires" step below is
> obsolete. The current smoke is: label the parent `minion-research` →
> verify the PRD + slice-plan comments → then label the parent
> `minion-approved` to produce one feature PR. A dry run of exactly this
> was done end-to-end on partio-io/cli#437 (→ PR #443). Not yet run
> against a real `minion-proposal` issue.

## What to build

This slice is operational, not code. Pick a small, well-scoped parent
issue on `partio-io/cli` and put the full `research.md` chain through
its first production run. The goal is to validate, before trusting
the auto-cascade for the long tail of complex tasks, that:

- The chain completes within the 90-minute timeout against a real,
  non-trivial parent issue.
- The PRD comment is substantive — a stranger reading just the parent
  issue thread can understand what's being built and why.
- The child issues are the right granularity (vertical, demoable,
  thin) and reference the parent.
- No personal data leaks from the persona substrate into any public
  artifact (PRD comment, child issue bodies, status comment, run
  logs).
- The parent issue stays open and is correctly labeled
  `minion-research-completed`.

The procedure intentionally pauses the auto-cascade for this first
run. After the publisher opens child issues with `minion-approved` +
`minion-research-output`, you remove the `minion-approved` label
manually from each child *before* `minion.yml` fires `implement.md`.
That gives a human review gate on the research output the very first
time, and only the very first time. Once the smoke is clean, future
runs trust the cascade end-to-end (the PRD's *Testing Decisions*
section is explicit: "After the first successful smoke run, future
runs trust the cascade.").

## Acceptance criteria

- [ ] Parent issue selected: a real `minion-proposal` issue from the
  cli queue, or a synthetic complex-task issue authored for the
  smoke. Recorded in the PR/notes attached to closing this slice.
- [ ] `minion-research` label applied to the chosen parent. Workflow
  log shows the run firing within seconds, passing the
  `author_association` gate, and cloning the external repo (master branch) successfully.
- [ ] Run completes inside the 90-minute timeout. Total wall-clock
  time recorded in the smoke notes.
- [ ] PRD comment present on the parent, substantive, and matches
  the shape produced by `/code-create-prd` (Problem Statement,
  Solution, User Stories, Implementation Decisions, Testing
  Decisions, Out of Scope, Further Notes — or the agreed
  near-equivalent).
- [ ] Child issues opened, each with `minion-approved` and
  `minion-research-output` labels and the
  `<!-- minion:slice parent=#<N> slice=<n>/<total> -->` marker.
- [ ] Status comment present on the parent listing all child issue
  numbers with the `minion:research-status` marker.
- [ ] Parent labeled `minion-research-completed`. Parent issue state
  is `open`. No `minion-done` label on the parent.
- [ ] Privacy review of every public artifact (PRD comment, every
  child body, status comment): no personal data, no quotes or
  paraphrases of TELOS or
  memory content.
- [ ] `minion-approved` removed from each child issue manually
  before `minion.yml` fires on it (this is the deliberate
  first-run gate). Smoke notes record which children would have
  fired.
- [ ] Smoke notes capture any persona blind spots, awkward question
  trees, or PRD-shape issues observed. These notes seed the future
  "tune-up gotchas" file (still out of scope here per PRD) and
  inform whether TELOS or memory updates are warranted (handled via
  the normal PR flow, not in this PR).

## Modules touched

- None — this slice runs the system end-to-end against existing
  artifacts and reports.

## Test prior art

- The PRD's *Testing Decisions / Modules to verify / End-to-end
  smoke* bullet describes this exact procedure.
- `partio-cli/.github/workflows/minion.yml` — example of the same
  runner / OAuth subscription auth path being exercised in
  production.

## Out of scope

- Building a CI assertion or scheduled test that scans research-run
  outputs for personal-data substrings. PRD: *Testing Decisions* —
  "Build only if a leak is observed in a real run."
- A persona "tune-up gotchas" file. PRD: *Out of Scope* — defer
  until persona misbehavior is observed.
- Automating the manual `minion-approved` strip. The strip is
  deliberately a one-time, first-smoke procedure; production runs
  should let the cascade fire.
- Any changes to `propose.yml`, `approve.yml` cron, or
  `author_association` on the existing `minion.yml`. PRD:
  *Out of Scope*.
