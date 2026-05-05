# 02 — Researcher + persona produce a Q&A transcript

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [01 — Workflow skeleton](./01-workflow-skeleton.md)

## What to build

Replace the stub agent in `research.md` with the first two real
sub-agents — `researcher` and `persona` — that together drive a
realistic research interview against the parent issue.

The `researcher` agent walks a decision tree in the style of the
`/code-research` skill, asking one question at a time and writing it
to `./research-transcript.md` in the worktree. It loops until it has
enough information to hand off to the PRD writer (introduced in slice
[#3](./03-prd-writer-comment-label.md)), at which point it writes a
trailing `RESEARCH_COMPLETE` marker on its own line.

The `persona` agent answers each unanswered question in the
transcript as jcleira would. Its prompt loads the full TELOS and
memory substrate from the argos clone created in slice
[#1](./01-workflow-skeleton.md) (`argos/telos/MISSION.md`,
`argos/telos/GOALS.md`, `argos/telos/PROJECTS.md`,
`argos/telos/BELIEFS.md`, all of `argos/memory/*.md` — currently 48
files, ~120K chars). The agent decides at runtime which substrate is
relevant per question; no curation or filtering happens at write
time.

The persona prompt contains a literal, hard-coded privacy directive
(see *Persona privacy directive* in the PRD). The directive is
load-bearing — every persona output must be in its own words, framed
as a decision, and must never quote or paraphrase health, training,
diary, financial, location, or calendar content from the substrate.

The two agents share `./research-transcript.md` and alternate: each
turn, the running agent reads the latest transcript state and
appends its block, then yields control. The agents loop until
`RESEARCH_COMPLETE`.

The publisher (still a single trailing agent at this slice) posts the
full transcript file as a comment on the parent issue so the user can
see what the researcher and persona produced. This comment is *not*
the final PRD — that arrives in slice
[#3](./03-prd-writer-comment-label.md). No idempotency marker is
required on this transitional comment.

## Acceptance criteria

- [ ] `research.md`'s `## Agents` section declares `researcher`,
  `persona`, and `publisher` agents in that sequential order.
- [ ] `research.md` has a `## Context` section with hints for
  `argos/telos/MISSION.md`, `argos/telos/GOALS.md`,
  `argos/telos/PROJECTS.md`, `argos/telos/BELIEFS.md`, and
  `argos/memory/*.md` (glob, not enumerated). The hints reference the
  paths under `${{ github.workspace }}/argos/` produced by slice
  [#1](./01-workflow-skeleton.md).
- [ ] The `persona` agent prompt contains a literal directive that
  reads, semantically: "Use TELOS and memory to *decide* — what
  answer would jcleira give? Never quote, paraphrase, or reference
  personal data (health, training, daily diary content, finances,
  location, calendar) in any output. Output answers in your own
  words, framed as decisions on the question at hand." Wording may be
  tuned over time, but the directive must be present and load-bearing
  in the prompt.
- [ ] Transcript flow: `researcher` writes one numbered question per
  turn to `./research-transcript.md`; `persona` reads the latest
  unanswered question, appends an `## Answer` block, then yields. The
  loop continues until `researcher` writes `RESEARCH_COMPLETE` on its
  own line.
- [ ] `publisher` reads `./research-transcript.md` and posts its
  contents as a comment on the parent issue via `gh issue comment`.
- [ ] On the same test issue used in slice
  [#1](./01-workflow-skeleton.md), a labeled re-run produces a
  comment containing realistic Q&A grounded in the parent issue's
  topic.
- [ ] Manual review of the comment confirms no personal data leak: no
  Whoop/Garmin numbers, no diary excerpts, no calendar event names,
  no financial figures, no specific location strings. The persona's
  answers read as *decisions*, not as recitations of substrate.

## Modules touched

- `partio-io/cli/.minions/programs/research.md` (extend)

No changes to `research.yml` (the workflow remains as built in slice
[#1](./01-workflow-skeleton.md)). No changes to `partio-minions` Go
runtime.

## Test prior art

- `~/.claude/skills/code-research/` (and the
  `/home/arvos/.claude/skills/code-create-prd/` skill) — the
  spiritual model for the researcher's interview style. The `argos`
  repo's `skills/code-research/SKILL.md` is the closest direct
  reference for question-tree shape.
- `partio-cli/.minions/programs/propose.md` — example of a program
  with a `## Context` section that lists files the agent should read.
- `partio-cli/.minions/programs/implement.md` — example of an agent
  block with `capabilities` (max_turns, retries). Multi-agent
  programs declare multiple `### <agent>` blocks under `## Agents`.

## Out of scope

- The `prd-writer` agent and PRD comment formatting. Covered in slice
  [#3](./03-prd-writer-comment-label.md).
- The `slicer` agent and child issue creation. Covered in slice
  [#4](./04-slicer-child-issues.md).
- Idempotency markers on the transcript comment. The transcript
  comment is transitional and will be replaced by the PRD comment in
  slice [#3](./03-prd-writer-comment-label.md).
- Automated regression tests scanning persona output for personal
  data. The PRD flags this as a possible follow-up only if a real
  leak is observed.
- A "tune-up gotchas" file. Defer until persona misbehavior is
  observed in real runs (PRD: *Out of Scope*).
