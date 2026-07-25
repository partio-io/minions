# 05 — Rollout

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [03 — Resume & idempotent re-run](./03-resume-idempotent-rerun.md), [04 — Failure comment](./04-failure-comment.md)

## What to build

Take the feature from merged-in-minions to live-in-production, in
deploy-chain order. The runtime change is inert until the cli repo's
workflows pin the new version — a tagged release with no pin bump
ships nothing.

Two artefacts:

1. **A minions release** — the runtime slices (01–04) tagged as a new
   version. Preparing release notes is in scope; creating the tag is
   the maintainer's action.
2. **One cli PR** — `implement.md` gains `slices: true` frontmatter
   plus per-slice agent instructions consistent with the "build only
   this slice" directive; the minion workflow raises
   `timeout-minutes` 30 → 60 and bumps the pinned minions version to
   the new release. Opened as a PR and left for review — never
   merged by the runtime or by argos.

Verification is one staged end-to-end build: a test issue in the cli
repo carrying a small hand-written slice plan (2 slices is enough),
triggered the normal way, producing one multi-commit PR with marker
commits, bounded per-slice sessions, and a clean done flow.

## User stories covered

20, 21 — and the production half of 16 (the real `implement.md` now
opts in).

## Acceptance criteria

- [x] Release notes for the new minions version summarize the
      slice-aware behavior and its backward compatibility.
- [x] The cli PR contains: `implement.md` opt-in + per-slice
      instructions, `timeout-minutes: 60`, and the version-pin bump —
      nothing else.
- [x] Order respected: pin bump PR references the tagged release; no
      pin points at an untagged ref.
- [x] Staged run on a test issue with a hand-written 2-slice plan
      completes: two slice sessions, marker commits on one branch,
      single PR, issue closed by the existing done flow.
- [x] A control run on a plan-less issue still builds single-session,
      confirming zero regression for unsliced issues.

## Modules touched

- cli repo: `implement.md` program, minion workflow file
  (timeout + version pin)
- minions repo: release notes only — no code

## Test prior art

- No unit tests here; verification is the staged end-to-end run plus
  the control run, per the PRD's testing decisions.

## Out of scope

- Any runtime code change — if the staged run exposes a bug, it goes
  back to the owning issue (01–04), and rollout repeats.
- Slicing for other programs, cross-repo plans, docs-repo coverage —
  out per PRD.

## Handoff

When this slice's preparable ACs pass (release notes written, cli PR
open), BEFORE flipping its row in `issues.md`, post the following
block to the user verbatim:

- **URL / artefact to visit**: the minions release-notes draft + the
  open cli PR (implement.md opt-in, timeout, pin bump)
- **Action required**: tag the minions release; review + merge the
  cli PR; then trigger the staged build on the prepared test issue
  and eyeball the resulting PR (marker commits, one branch, clean
  done flow)
- **Where to record the decision**:
  - In `issues.md` (add a one-line note next to this slice's row with
    the release tag and staged-run PR link)
  - No following slice — this note closes the feature.

The feature is not "done" until the staged run's outcome is recorded.

Prepared artefacts (2026-07-24): the runtime slices were never
committed, so rollout also opened the feature PR itself.

- Feature PR (slices 01–04 + docs): partio-io/minions#141
- Release-notes draft: v0.0.11 (publish creates the tag; after #141)
- cli rollout PR: partio-io/cli#563 (opt-in, timeout 60, pin v0.0.11)
- Staged-run issues (inert until labeled `minion-approved`):
  partio-io/cli#561 (2-slice plan), partio-io/cli#562 (plan-less
  control)

Staged-run outcome (2026-07-25): v0.0.11 tagged at the #141 merge
commit and live via cli#563.

- Staged run (cli#561): first attempt failed in `go install` —
  sum.golang.org returned 500 on the minutes-old tag (checksum-db
  ingestion lag; nothing was built). The retry built both slices
  green — work commit + `minion:slice N/2` marker each, one branch —
  but slice 1's session opened the PR itself (cli#567), so the
  runtime's PR step collided and the run went red. A third trigger
  proved resume in production: fetched markers 2/2, ran zero
  sessions, adopted cli#567 via ensurePRs, and the done flow closed
  the issue.
- Control run (cli#562): the plan-less path stayed single-session
  with no marker commits. Same agent-opened-PR collision on the
  first attempt (cli#568, closed) — caused by the test issue's own
  body promising "one PR", which the agent dutifully implemented.
  With that phrase removed, the re-run was fully green:
  runtime-created cli#569, done flow closed the issue.
- Follow-ups worth their own issues (runtime scope, not rollout):
  (1) sessions run with `gh` and bypassPermissions will open a PR
  whenever the issue text mentions one — the runtime's agent prompt
  should forbid it, since program-body sections beyond the intro are
  not injected into the session prompt; (2) the single-session PR
  step could adopt an existing open PR for its branch the way the
  slice path's ensurePRs already does.
