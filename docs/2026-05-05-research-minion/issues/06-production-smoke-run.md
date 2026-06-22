# 06 — First production smoke run

**Blocked by**: [#8 — Persona substrate: in-repo telos+memory](./08-persona-local-substrate.md)

> Operational, not code. Validates the full `research.md` chain
> end-to-end on a real issue, once the in-repo substrate (#8) is in place.

## What to do

Pick a small, well-scoped parent issue on `partio-io/cli` and put the
full `research.md` chain through its first production run. The current
procedure (no auto-cascade):

1. Label the parent `minion-research` (or comment `/minion research`).
2. Verify the run posts a substantive **PRD comment** and a **slice-plan
   comment**, and labels the parent `minion-research-completed` (the
   parent stays open).
3. After review, label the parent `minion-approved` (or comment
   `/minion build`) to produce a single feature PR.

A dry run of this flow was exercised end-to-end on partio-io/cli#437
(→ PR #443); this slice is the first run against a real research issue.

## Acceptance criteria

- [ ] Parent issue selected (a real cli issue or a synthetic complex
  task), recorded in the closing notes.
- [ ] `minion-research` applied; the run fires, passes the
  `author_association` gate, and completes within the 90-minute timeout.
- [ ] A substantive **PRD comment** is posted, matching the
  `/code-create-prd` shape, grounded in the in-repo substrate.
- [ ] A **slice-plan comment** is posted with the proposed vertical
  slices. No child issues are created.
- [ ] Parent labeled `minion-research-completed`; parent state `open`;
  no `minion-done`.
- [ ] Privacy review of every public comment: no personal data, and no
  quotes or paraphrases of substrate content.
- [ ] Labeling the parent `minion-approved` produces exactly one feature
  PR.
- [ ] Smoke notes capture any persona blind spots or PRD-shape issues
  observed.

## Modules touched

- None — runs the system end-to-end and reports.

## Out of scope

- A CI assertion that scans research-run outputs for personal-data
  substrings (build only if a leak is observed in a real run).
- Any changes to the existing `minion.yml` cascade.
