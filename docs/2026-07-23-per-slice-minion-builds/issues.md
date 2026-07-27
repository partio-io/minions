# Issues — 2026-07-23-per-slice-minion-builds

Source: [prd.md](./prd.md)

| Done | # | Title | Blocked by |
|------|---|-------|------------|
| [x]  | 1 | [Dry-run walking skeleton](./issues/01-dry-run-walking-skeleton.md) | None |
| [x]  | 2 | [Live slice loop](./issues/02-live-slice-loop.md) | [#1](./issues/01-dry-run-walking-skeleton.md) |
| [x]  | 3 | [Resume & idempotent re-run](./issues/03-resume-idempotent-rerun.md) | [#2](./issues/02-live-slice-loop.md) |
| [x]  | 4 | [Failure comment](./issues/04-failure-comment.md) | [#2](./issues/02-live-slice-loop.md) |
| [x]  | 5 | [Rollout](./issues/05-rollout.md) | [#3](./issues/03-resume-idempotent-rerun.md), [#4](./issues/04-failure-comment.md) |

Slice 5 note (2026-07-25): v0.0.11 released and pinned (cli#563).
Staged run: cli#567 (two slice sessions, markers, resume-adopted PR).
Control: cli#569 (single-session, no markers). Feature closed.
