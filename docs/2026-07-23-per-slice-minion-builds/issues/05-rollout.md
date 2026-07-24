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

- [ ] Release notes for the new minions version summarize the
      slice-aware behavior and its backward compatibility.
- [ ] The cli PR contains: `implement.md` opt-in + per-slice
      instructions, `timeout-minutes: 60`, and the version-pin bump —
      nothing else.
- [ ] Order respected: pin bump PR references the tagged release; no
      pin points at an untagged ref.
- [ ] Staged run on a test issue with a hand-written 2-slice plan
      completes: two slice sessions, marker commits on one branch,
      single PR, issue closed by the existing done flow.
- [ ] A control run on a plan-less issue still builds single-session,
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
