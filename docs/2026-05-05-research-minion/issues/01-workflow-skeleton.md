# 01 — Workflow skeleton: label fires workflow, posts a stub comment

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately

## What to build

A new minion path on `partio-io/cli` that fires when a parent issue is
labeled `minion-research` (or commented `/minion research`). The
workflow clones the private source repo into the runner workspace, then runs
`minions run .minions/programs/research.md --issue <N>`. At this slice,
`research.md` contains a single trivial agent that uses `gh` to post a
"research started — run-id `<sha7>`" comment on the parent issue and
exit. No PRD, no children, no further sub-agents yet.

This is the tracer bullet: it proves the wire end-to-end (label →
workflow trigger → author_association gate → private-source clone via PAT →
minions runtime → multi-agent program parsing → gh side effect on
issue) before any real research logic is added.

The existing simple-task flow on `minion.yml` and the `propose.yml`
cron are not modified.

## Acceptance criteria

- [ ] Three new labels exist on `partio-io/cli`: `minion-research`,
  `minion-research-output`, `minion-research-completed`. Colors and
  descriptions follow the existing minion-label convention in the
  repo.
- [ ] `secrets.GH_PAT` on `partio-io/cli` is scoped to read
  the private source repo (private repo). Verified by a successful clone in a
  workflow run.
- [ ] `.github/workflows/research.yml` exists on `partio-io/cli` with:
  - Triggers: `issues.labeled` (gated to `minion-research`), and
    `issue_comment.created` (gated to body containing
    `/minion research`).
  - Job-level `if:` gate enforcing
    `github.event.issue.author_association` (or
    `github.event.comment.author_association`) in
    `[OWNER, MEMBER, COLLABORATOR]`. Triggers from anyone else are
    silently skipped.
  - `runs-on: github-runner-partio-minion-ai-01` (same self-hosted
    runner as `minion.yml`).
  - `timeout-minutes: 90`.
  - A step that clones the private source repo (master
    branch) into `${{ github.workspace }}/private-source/` using
    `secrets.GH_PAT`.
  - Reuses the existing `Install minions` step pattern (go install
    `github.com/partio-io/minions/cmd/minions@<pinned-version>`).
  - A step that runs `minions run .minions/programs/research.md
    --issue ${{ github.event.issue.number }}` with `GH_TOKEN:
    secrets.GH_PAT`.
  - A `failure()` step that adds `minion-failed` label and posts a
    comment with the run URL — same pattern as `minion.yml`'s "Mark
    failed" step.
  - **No** "Mark done" / auto-close step (the parent must stay open
    after a successful research run).
- [ ] `.minions/programs/research.md` exists with:
  - Frontmatter declaring `id: research`, `target_repos: [cli]`. No
    `acceptance_criteria` and no `pr_labels` (the run produces no PR).
  - A single `## Agents` block containing one agent (e.g., `stub`)
    whose prompt uses `gh issue comment` to post a one-line "research
    started — run-id `<sha7>`" message on the parent issue, where
    `<sha7>` is derived from the issue's events or the workflow run
    id.
- [ ] Smoke run: a test issue on `partio-io/cli` labeled
  `minion-research` causes the workflow to fire, the workflow log
  shows the private-source clone step succeeding (and prints the private-source SHA),
  and a comment from the runner appears on the parent issue.
- [ ] `minion.yml`, `propose.yml`, `propose.md`, `implement.md`,
  `approve.md`, `doc-update.md`, `readme-update.md`, `ingest.md` are
  not modified. The Go binary in `partio-minions` is not modified.

## Modules touched

- `partio-io/cli/.github/workflows/research.yml` (new)
- `partio-io/cli/.minions/programs/research.md` (new, stub form)
- `partio-io/cli` repo labels (3 new)
- `secrets.GH_PAT` configuration on `partio-io/cli` (manual GitHub UI
  action)

No changes to `github.com/partio-io/minions` (the Go runtime). This
slice is purely additive at the workflow / program / config layer.

## Test prior art

- `partio-io/cli/.github/workflows/minion.yml` — model the new
  workflow after this one. Specifically, mirror its "Install
  minions", "Run program", and "Mark failed" step shape; diverge only
  on the items called out in the PRD's
  *Implementation Decisions / Module interfaces* section.
- `partio-io/cli/.minions/programs/propose.md` and `approve.md` —
  examples of programs that produce only `gh` side effects with no PR
  output. No `pr_labels` and no `acceptance_criteria` in their
  frontmatter; the run is treated as "no worktree changes" and is
  marked skipped, not failed.
- `partio-minions/internal/program/parse_test.go` — running
  `minions run research.md --dry-run` exercises this parser and is
  the only program-file validation needed.

## Out of scope

- Researcher / persona / prd-writer / slicer agents and the privacy
  directive. Covered in slice [#2](./02-researcher-persona-transcript.md)
  and beyond.
- Idempotency markers on the stub comment. The stub comment is a
  smoke marker only and will be replaced in slice
  [#3](./03-prd-writer-comment-label.md).
- A real PRD comment, child issues, or
  `minion-research-completed` labeling — slices
  [#3](./03-prd-writer-comment-label.md) and
  [#4](./04-slicer-child-issues.md).
- Tightening `author_association` on existing `minion.yml`. The PRD
  flags this as worth doing eventually; it is NOT part of this slice.
