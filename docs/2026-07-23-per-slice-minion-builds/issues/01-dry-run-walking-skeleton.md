# 01 — Dry-run walking skeleton

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately

## What to build

Running the implement program with `--dry-run` against an issue whose
discussion carries a `minion:research-slices` comment prints N
per-slice prompts instead of today's single whole-issue prompt. Each
printed prompt contains the issue title and body, the PRD comment (the
`minion:research` comment, when present), the full slice plan for
orientation, and its own target slice singled out with an explicit
"build only this slice" directive.

The path is gated twice: the program must opt in via a `slices: true`
frontmatter flag, and the issue must actually carry a parseable plan.
A malformed plan (missing marker, numbering gap, a slice without
acceptance criteria) fails the run with a descriptive parse error —
never a silent fallback to the whole-issue prompt. An issue with no
slice-plan comment, or a program without the flag, dry-runs exactly as
today.

This slice is prompt-generation only: no Claude sessions, no
worktrees, no branches, no PRs. It proves the whole plumbing path —
issue comments fetched and passed structurally, plan parsed, executor
expanding one implement agent into N slice prompts.

## User stories covered

3, 13, 14, 15, 16, 17 — and the prompt-side half of 1 (bounded
per-slice context; the session-per-slice half lands in issue 02).

## Acceptance criteria

- [x] A new `slices` module parses a slice-plan comment (the
      publisher's `minion:research-slices` marker followed by
      `### Slice <n> — <Title>` sections) into an ordered plan with
      title, description, acceptance criteria, and, when present,
      modules touched and out of scope per slice.
- [x] A hand-written comment in the same marker format parses
      identically to a publisher-produced one.
- [x] Malformed plans (no marker, numbering gap, empty acceptance
      criteria) return descriptive errors naming what is wrong; a
      slice-aware run surfaces that error and fails — no fallback.
- [x] Programs gain a `slices: true` frontmatter flag; only flagged
      programs take the slice-aware path.
- [x] The run command passes fetched issue comments to the executor
      structurally, alongside the existing rendered context blob.
- [x] `--dry-run` on a flagged program + plan-bearing issue prints one
      prompt per slice; each prompt contains issue body, PRD comment,
      full plan, and the "build only this slice" directive naming its
      slice.
- [x] `--dry-run` with no plan comment, or on an unflagged program,
      prints today's single prompt unchanged.
- [x] All new behavior covered by tests: parser fixtures (publisher
      format, hand-written, malformed variants), frontmatter parsing,
      and per-slice prompt composition.

## Modules touched

- `slices` (new) — plan parsing + validation (resume-point half comes
  in issue 03)
- `program` — frontmatter opt-in flag
- `run` command — structured comment plumbing
- `executor` — dry-run expansion and per-slice prompt composition

## Test prior art

- Table-driven parsing tests with inline fixtures: the run command's
  issue-context rendering tests (comment selection, noise filtering).
- Frontmatter parsing tests in the program package.
- Executor dry-run behavior in the executor package's tests.

## Out of scope

- Running any Claude session, creating worktrees, branches, commits,
  or PRs — issue 02.
- Resume-point computation from branch markers — issue 03.
- Changing `implement.md` in the cli repo — issue 05 (local fixture
  program files are enough here).
