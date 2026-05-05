# 03 — prd-writer agent + PRD comment + parent label

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [02 — Researcher + persona produce a Q&A transcript](./02-researcher-persona-transcript.md)

## What to build

Add a `prd-writer` sub-agent that runs after `researcher` + `persona`
have produced `./research-transcript.md`. The writer reads the
completed transcript and synthesizes a PRD body — the same shape the
`/code-create-prd` skill produces (Problem Statement, Solution, User
Stories, Implementation Decisions, Testing Decisions, Out of Scope,
Further Notes). Output goes to `./prd-draft.md` in the worktree.

The `publisher` agent (still trailing the chain) is upgraded:
instead of posting the transcript as a comment, it now posts the
contents of `./prd-draft.md` as a single comment on the parent issue.
The first line of that comment is the idempotency marker
`<!-- minion:research run-id=<sha7> -->`, where `<sha7>` is derived
from the workflow's `github.run_id` or the issue's events (any stable
per-run identifier).

After posting the PRD comment, the publisher adds the
`minion-research-completed` label to the parent issue. The publisher
explicitly does NOT add `minion-done` and does NOT close the parent —
the parent stays open until jcleira closes it manually.

After this slice, a labeled run produces: real PRD body comment with
marker, parent labeled `minion-research-completed`, parent open. Still
no child issues — those arrive in slice
[#4](./04-slicer-child-issues.md).

## Acceptance criteria

- [ ] `research.md`'s `## Agents` section declares `researcher`,
  `persona`, `prd-writer`, `publisher` in that sequential order.
- [ ] `prd-writer` reads `./research-transcript.md` and writes
  `./prd-draft.md` in the worktree. The output follows the
  `/code-create-prd` shape (Problem Statement, Solution, User
  Stories, Implementation Decisions, Testing Decisions, Out of
  Scope, Further Notes — section headings may be tuned but the
  general structure must hold).
- [ ] `publisher` posts `./prd-draft.md` content as a comment on the
  parent issue via `gh issue comment`. The first line of the comment
  body is exactly `<!-- minion:research run-id=<sha7> -->`, where
  `<sha7>` is a stable seven-character identifier derived from the
  current run.
- [ ] `publisher` adds the `minion-research-completed` label to the
  parent issue via `gh issue edit --add-label`.
- [ ] `publisher` does NOT add `minion-done` to the parent and does
  NOT call `gh issue close` on the parent.
- [ ] The transcript-as-comment behavior from slice
  [#2](./02-researcher-persona-transcript.md) is removed (replaced by
  the PRD comment).
- [ ] The privacy directive from slice
  [#2](./02-researcher-persona-transcript.md) remains intact in the
  `persona` agent prompt.
- [ ] Manual review of the comment on the test issue confirms: a
  real PRD body, the marker on line 1, parent labeled
  `minion-research-completed`, parent issue state still `open`, no
  personal data leaks.

## Modules touched

- `partio-io/cli/.minions/programs/research.md` (extend)

No changes to `research.yml`. No changes to `partio-minions` Go
runtime.

## Test prior art

- `~/.claude/skills/code-create-prd/` — the spiritual model for the
  PRD shape and section headings.
- The PRD this issue file ships against
  ([../prd.md](../prd.md)) is itself the canonical example of the
  output shape.
- `partio-cli/.minions/programs/propose.md` — pattern for an agent
  prompt that uses `gh` commands directly to mutate GitHub state.

## Out of scope

- The `slicer` agent, child issue creation, status comment on parent.
  Covered in slice [#4](./04-slicer-child-issues.md).
- Skip-if-marker-exists logic on the PRD comment. Markers are written
  in this slice; the *check before write* logic lives in slice
  [#5](./05-idempotency-rerun-skip.md).
- PRD-comment versioning / history. PRD: *Out of Scope* — first
  version is what's posted.
- Automated parent-close when child PRs are merged. PRD: *Out of
  Scope*.
