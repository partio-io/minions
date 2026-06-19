# Issues — 2026-05-05-research-minion

Source: [prd.md](./prd.md)

| Done | # | Title | Blocked by |
|------|---|-------|------------|
| [x]  | 1 | [Workflow skeleton — label fires workflow, posts a stub comment](./issues/01-workflow-skeleton.md) | None |
| [x]  | 2 | [Researcher + persona produce a Q&A transcript](./issues/02-researcher-persona-transcript.md) | [#1](./issues/01-workflow-skeleton.md) |
| [x]  | 3 | [prd-writer agent + PRD comment + parent label](./issues/03-prd-writer-comment-label.md) | [#2](./issues/02-researcher-persona-transcript.md) |
| [x]  | 4 | [slicer agent + child issues + status comment](./issues/04-slicer-child-issues.md) — **superseded by #7** | [#3](./issues/03-prd-writer-comment-label.md) |
| [x]  | 7 | [Slice plan as a comment + manual one-PR build (replaces #4)](./issues/07-slice-plan-comment-manual-build.md) | [#3](./issues/03-prd-writer-comment-label.md) |
| [ ]  | 5 | [Idempotency — re-runs skip existing artifacts](./issues/05-idempotency-rerun-skip.md) — *premise revised by #7* | [#7](./issues/07-slice-plan-comment-manual-build.md) |
| [ ]  | 8 | [Persona substrate: in-repo telos+memory, drop the private source repo](./issues/08-persona-local-substrate.md) — **private-source clone already cut (cli#454)**; substrate TODO | None |
| [ ]  | 6 | [First production smoke run](./issues/06-production-smoke-run.md) — *procedure revised by #7; also blocked by #8* | [#8](./issues/08-persona-local-substrate.md) |
