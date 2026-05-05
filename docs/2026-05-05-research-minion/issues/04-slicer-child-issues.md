# 04 — slicer agent + child issues + status comment

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [03 — prd-writer agent + PRD comment + parent label](./03-prd-writer-comment-label.md)

## What to build

Add a `slicer` sub-agent that runs between `prd-writer` and
`publisher`. The slicer reads `./prd-draft.md` (produced in slice
[#3](./03-prd-writer-comment-label.md)) and decomposes the PRD into
vertical-slice descriptions, one block per slice. Output goes to
`./slices.md` in the worktree. Each slice block contains: title,
description (1–3 paragraphs of end-to-end behavior), acceptance
criteria checklist, modules touched, and out-of-scope pointers — the
same shape this issue file follows.

The `publisher` is extended again. After posting the PRD comment and
labeling the parent (slice [#3](./03-prd-writer-comment-label.md)), it
now also opens one GitHub issue per slice on the same repo via
`gh issue create`. Each child issue:

- Is created with two labels: `minion-approved` (the existing trigger
  for `minion.yml`, which fires `implement.md` automatically) and
  `minion-research-output` (provenance, for filtering).
- Has its body prefixed by exactly
  `<!-- minion:slice parent=#<N> slice=<n>/<total> -->` on line 1,
  where `<N>` is the parent issue number, `<n>` is the 1-indexed
  slice ordinal, and `<total>` is the total slice count for this run.
- Has a title derived from the slice's title field in `./slices.md`.
- Has a body that includes the slice description, acceptance
  criteria, modules touched, and a back-reference to the parent issue
  (`Parent: #<N>`) so a fresh `implement.md` run on the child has
  enough context.

After all child issues are open, the publisher posts a final status
comment on the parent issue listing the child issue numbers as
clickable references (`#<child-N>`). The first line of that comment
is exactly `<!-- minion:research-status parent=#<N> -->`.

The auto-cascade is deliberate: each child issue's `minion-approved`
label causes `minion.yml` to fire `implement.md` on that child, which
opens a PR. No new wiring is needed between research and implement —
they compose via the existing label.

## Acceptance criteria

- [ ] `research.md`'s `## Agents` section declares `researcher`,
  `persona`, `prd-writer`, `slicer`, `publisher` in that sequential
  order.
- [ ] `slicer` reads `./prd-draft.md` and writes `./slices.md`. Each
  slice block contains at minimum: title, description, acceptance
  criteria checklist.
- [ ] `publisher` opens one child issue per slice on
  `partio-io/cli` via `gh issue create`, each with labels
  `minion-approved` AND `minion-research-output`.
- [ ] Each child issue body's first line is exactly
  `<!-- minion:slice parent=#<N> slice=<n>/<total> -->` with the
  correct values substituted.
- [ ] Each child issue body contains the slice description,
  acceptance criteria, modules touched, and a `Parent: #<N>`
  back-reference.
- [ ] `publisher` posts a final status comment on the parent issue
  with first line exactly `<!-- minion:research-status parent=#<N> -->`,
  and a body listing each child issue as `#<child-N>`.
- [ ] Smoke run on the test issue from prior slices: PRD comment
  appears, then N child issues open, then the status comment appears
  listing those N children. Each child receives both labels.
- [ ] Auto-cascade verification: at least one child issue is
  observed to trigger `minion.yml` and produce a PR via
  `implement.md` (or, if testing in isolation, the
  `minion-approved` label is removed manually before the cascade
  fires — mirroring the smoke procedure planned in slice
  [#6](./06-production-smoke-run.md)).
- [ ] Existing simple-task flow (`minion.yml` + `implement.md`) is
  unchanged.

## Modules touched

- `partio-io/cli/.minions/programs/research.md` (extend)

No changes to `research.yml`. No changes to `minion.yml`,
`implement.md`, or any other existing program. No changes to
`partio-minions` Go runtime.

## Test prior art

- `~/.claude/skills/code-create-issues/` — the spiritual model for
  the slicer's vertical-slice shape.
- This issue file (and its siblings under
  `partio-minions/docs/2026-05-05-research-minion/issues/`) is
  itself a worked example of the slice block shape the slicer should
  emit.
- `partio-cli/.minions/programs/propose.md` — pattern for using
  `gh issue create` with labels from inside an agent prompt. The
  propose program also writes a body marker
  (`<!-- program: .minions/programs/<id>.md -->`) — the same idea,
  same line-1 convention.

## Out of scope

- Skip-if-marker-exists logic on the PRD comment, child issues, and
  status comment. Markers are written here; the *check before write*
  logic lives in slice [#5](./05-idempotency-rerun-skip.md).
- The first production smoke run against a real `minion-proposal`
  issue. Covered in slice [#6](./06-production-smoke-run.md).
- A failure-recovery path beyond the `minion-failed` label + run-URL
  comment inherited from slice [#1](./01-workflow-skeleton.md). PRD:
  *Out of Scope*.
- Automated dashboards for child-PR success rate. PRD: *Out of Scope*.
