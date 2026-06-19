# research-minion

> **Design revised (2026-06) — shipped.** This PRD was written for a
> *child-issue* design: the publisher opened one GitHub issue per slice,
> each pre-labeled `minion-approved`, so `minion.yml` cascaded
> `implement.md` per child. That was built (slice #4) and then
> **replaced** — it proliferated issues and split one feature across many
> PRs. The shipped design instead:
> - **Research output is comments only.** The publisher posts the PRD as
>   one comment and the slice plan as a second comment on the parent;
>   it opens **no** child issues and applies **no** `minion-approved`.
> - **Implementation is a manual, single-PR step.** jcleira labels the
>   parent `minion-approved` (or `/minion build`) when ready; `implement.md`
>   runs once on the parent and produces **one** feature PR.
> - Each build gets its **own** branch `minion/<prog>-<agent>-<issue>`
>   (minions ≥ v0.0.6), so builds never collide on a shared branch/PR.
>
> Shipped via partio-io/cli#436, partio-io/minions#130 + release v0.0.6,
> and partio-io/cli#439. The current design is slice
> [#07](./issues/07-slice-plan-comment-manual-build.md). Sections below
> that describe child issues or the auto-cascade (Solution, several User
> Stories, Idempotency contract, Build/Auto-cascade trigger semantics,
> Specific interactions) predate this revision and are kept for history.

## Problem Statement

The current minion system handles small, well-specified tasks well: a human files an issue on `partio-io/cli`, labels it `minion-approved` (or comments `/minion build`), and `implement.md` runs unattended to produce a PR. That fast path is sufficient when the spec is tight and the change is small.

It is not sufficient for complex work. Many of the proposals sitting in the cli queue (270+ open `minion-proposal` issues, plus ad-hoc complex tasks jcleira wants to tackle) require research and decomposition before implementation. Today, jcleira handles that complexity interactively through the `/code-research` → `/code-create-prd` → `/code-create-issues` → `/code-execute-issue` skill chain — each step requires him at the keyboard, answering questions and making decisions.

That contradicts the stated goal: "many one-shot tasks per day." If every complex task pulls jcleira into a 30-minute interactive interview, the system caps at the rate jcleira can sit and interview — far below the rate at which complex tasks accumulate.

The system needs a path that fires complex tasks the same way it fires simple ones: tag the issue, walk away, get PRs back. That path must produce decisions of the same quality jcleira would produce live, while consuming none of his interactive time.

## Solution

A new minion path for complex tasks runs the full research → PRD → slice → implement pipeline unattended.

A new GitHub label `minion-research` (or comment `/minion research`) on a parent issue triggers a new workflow `research.yml` on partio-io/cli. That workflow fires `research.md`, a multi-agent program with five sub-agents:

1. **researcher** — drives an interview in the style of `/code-research`, walking the decision tree and writing Q&A to a transcript file in the worktree.
2. **persona** — answers each question as jcleira would, grounded in the private source repo's full `telos/*.md` (~5K chars) and `memory/*.md` (~120K chars across 48 files). Cloned fresh from the private source repo (master branch) into the workspace at workflow start. Strict directive in the prompt: use the substrate to *decide*, never quote or paraphrase personal content in any output.
3. **prd-writer** — synthesizes the transcript into a PRD body.
4. **slicer** — breaks the PRD into vertical-slice descriptions, one block per slice.
5. **publisher** — uses the `gh` CLI to: post the PRD as a comment on the parent issue, post the slice plan as a second comment, and label the parent `minion-research-completed`. (Originally: opened one child issue per slice plus a status comment — replaced; see the Design-revised banner above.)

The research run produces review artifacts only — no child issues, no `minion-approved`, no implementation. When jcleira is ready he labels the parent `minion-approved` (or comments `/minion build`); `minion.yml` then fires `implement.md` once on the parent and produces a single feature PR. The manual gate is deliberate: jcleira reviews the PRD + slice plan before any code is written.

All artifacts stay on GitHub Issues and Pull Requests; no `docs/.../prd.md` or `docs/.../issues/*.md` files get committed. Idempotency markers (`<!-- minion:research run-id=<sha> -->` on the PRD comment, `<!-- minion:slice parent=#N -->` on child issue bodies) let re-runs detect existing artifacts and avoid duplication.

The existing simple-task flow is preserved unchanged. The propose pipeline (twice-daily cron generating `minion-proposal` issues from competitor monitoring) keeps running. New labels and the new workflow live alongside existing ones.

## User Stories

1. As jcleira, I want to mark a complex partio-io/cli issue with `minion-research`, so that the system runs research, decomposition, and implementation without further intervention from me.
2. As jcleira, I want a "persona" sub-agent that decides as I would, so that the unattended research phase produces the same answers I'd give if I were at the keyboard.
3. As jcleira, I want the persona grounded in my full `telos/*.md` and all 48 `memory/*.md` files, so that decisions reflect my current goals, projects, beliefs, and learned preferences without me writing a separate persona file.
4. As jcleira, I want the persona to use TELOS/memory only for decision-making and never to quote, paraphrase, or reference personal data in any output, so that public PRs and issue comments don't leak my health, training, financial, or location information.
5. As jcleira, I want each minion phase (research, PRD synthesis, slice creation, slice implementation) to run as its own one-shot Claude session within the multi-agent program, so that each phase has clean context and the system stays composable.
6. As jcleira, I want the research minion to post the PRD as a comment on the parent issue rather than commit it as a doc file, so that the entire chain stays on GitHub Issues and PRs without producing extra `docs/` artifacts.
7. As jcleira, I want the research minion to open child issues directly via `gh` (not commit issue files), so that humans and the existing `minion.yml` workflow see the same artifacts in the same place.
8. As jcleira, I want each child issue to carry both `minion-approved` (to auto-fire implement.md) and `minion-research-output` (for provenance filtering), so that the auto-cascade works and so that I can identify which issues came from research runs at a glance.
9. As jcleira, I want the parent research issue to receive a `minion-research-completed` label and a status comment listing child issue links, so that the relationship between parent proposal and child slices is human-readable in the issue thread.
10. As jcleira, I want the parent research issue to stay open (not auto-close, no `minion-done` label) until I decide it's resolved, so that the issue serves as a hub for the work it spawned.
11. As jcleira, I want the existing simple-task flow (`minion-approved` label fires `implement.md` directly) to remain untouched, so that I can keep using the fast path for tasks I judge to be small.
12. As jcleira, I want to be able to triage a `minion-proposal` issue into either `minion-approved` (simple, direct implement) or `minion-research` (complex, full pipeline) based on my judgment, so that I have a fast path and a thorough path on the same proposal queue.
13. As jcleira, I want to run the research minion locally first (via `minions run .minions/programs/research.md --dry-run` with a workspace cloned side-by-side) before trusting CI, so that I can debug a new program without fighting CI feedback delay.
14. As jcleira, I want the research workflow to clone the private source repo at runtime via the PAT, so that the persona always reads the latest TELOS and memory state and so that any future runner (laptop replacement, VM, hosted runner) works without additional setup.
15. As jcleira, I want the workflow to clone the private source repo's `master` branch (since the private source repo uses `master`, not `main`), so that the clone step doesn't fail silently on a partio-io-conventional `main` assumption.
16. As jcleira, I want the research workflow to enforce `author_association in [OWNER, MEMBER, COLLABORATOR]` before firing, so that random commenters on the public cli repo cannot trigger research runs that consume my OAuth subscription quota.
17. As jcleira, I want a 90-minute workflow timeout (vs the existing 30 minutes on `minion.yml`), so that long research + PRD + slicing runs do not get killed mid-flight.
18. As jcleira, I want minion-authored issue comments prefixed with `<!-- minion:research run-id=<sha7> -->` and child issue bodies prefixed with `<!-- minion:slice parent=#N -->`, so that re-runs of `research.md` against the same parent issue can detect existing artifacts and avoid duplicating work.
19. As jcleira, I want the slicer agent to check for an existing PRD comment (by marker) before writing a new one, so that re-running `research.md` doesn't post a second PRD or open duplicate child issues.
20. As jcleira, I want the persona substrate to load all of `telos/*.md` and all of `memory/*.md` without filtering, so that the persona has full grounding without me curating which files are "important enough."
21. As jcleira, I want the research minion to share the same self-hosted runner, OAuth subscription auth, and PAT setup as the existing propose/implement minions, so that I don't introduce a new auth surface or new infrastructure to maintain.
22. As jcleira, I want `secrets.GH_PAT` on partio-io/cli scoped so it can read the private source repo, so that the workflow's clone step succeeds.
23. As jcleira, I want the research workflow to NOT auto-close the parent issue or add `minion-done` (unlike `minion.yml`'s success path), so that the parent stays open until I close it manually.
24. As jcleira, I want to retain the option to add a "tune-up gotchas" file later (without rebuilding the system), so that I can correct the persona's blind spots when I see them in real runs.
25. As jcleira, I want the propose loop to keep running unchanged (twice-daily cron generating `minion-proposal` issues), so that competitor-monitoring continues while I rebuild engagement with the system.
26. As jcleira, I want minion-failed signals (existing `minion-failed` label + comment with run URL) reused for research failures, so that I have a consistent observability story across all minion paths.
27. As jcleira, I want PR review to remain the only manual gate downstream of `minion-research` approval, so that I can fire complex tasks and walk away exactly the way I fire simple ones.
28. As future jcleira (or successor), I want this design captured as a PRD so that decisions about persona substrate, idempotency, and chain semantics aren't reconstructed from scratch when adjustments become necessary.
29. As jcleira, I want no code change to the `partio-minions` Go binary, so that the runtime stays simple and this feature is purely additive at the program / workflow / config layer.
30. As jcleira, I want the privacy directive to be hard-coded in `research.md`'s persona-agent prompt (not a separate convention I have to remember), so that the protection is load-bearing and tested every run rather than relying on author discipline.

## Implementation Decisions

### Modules built or modified

- **`research.md` program** (new, in `partio-io/cli/.minions/programs/`). Multi-agent program with five sub-agents executed sequentially in a single minion run: researcher, persona, prd-writer, slicer, publisher. Each agent has its own prompt; they share a worktree and pass state via files (transcript, draft PRD, slice list). Frontmatter declares `target_repos: [cli]`, no acceptance criteria (no code is written by the research run itself), and no `pr_labels` (no PR is created).
- **`research.yml` workflow** (new, in `partio-io/cli/.github/workflows/`). Mirrors `minion.yml`'s structure with these differences: triggers on `issues.labeled` for `minion-research` and on `issue_comment` containing `/minion research`; gates on `author_association` in `[OWNER, MEMBER, COLLABORATOR]`; clones the private source repo (master) into the workspace before invoking minions; 90-minute timeout; omits the success-path "Mark done" / auto-close steps.
- **GitHub labels on `partio-io/cli`** (new): `minion-research` (input trigger), `minion-research-output` (provenance on slicer-created child issues), `minion-research-completed` (parent state after research run succeeds).
- **`secrets.GH_PAT`** scope expansion to grant read access to the private source repo. Configuration change, no file change.
- **No changes to `partio-minions`**. The Go binary already supports multi-agent programs, sequential execution, context hints, and gh-CLI side effects from agent prompts. `research.md` is a new consumer of that runtime, not an extension of it.
- **No changes to `implement.md`, `minion.yml`, `propose.md`, `propose.yml`, `approve.md`, `doc-update.md`, `readme-update.md`, `ingest.md`**. Existing simple-task flow and propose pipeline run untouched.

### Module interfaces (program-level, not file-level)

- **researcher → persona handoff**: researcher writes one question at a time to a transcript file (`./research-transcript.md`); persona reads the latest unanswered question, appends an answer block. Both agents loop over the same file until researcher emits a `RESEARCH_COMPLETE` marker.
- **prd-writer reads** the completed transcript, writes a draft PRD to `./prd-draft.md`.
- **slicer reads** `./prd-draft.md`, writes a slice list to `./slices.md` (one block per slice with title, description, acceptance criteria).
- **publisher reads** `./prd-draft.md` and `./slices.md`, executes `gh` calls to post the PRD comment, open child issues, and label the parent. All `gh`-emitted artifacts include idempotency markers.

### Persona privacy directive

The persona-agent's prompt contains a literal, hard-coded directive: "Use TELOS and memory to *decide* — what answer would jcleira give? Never quote, paraphrase, or reference personal data (health, training, daily diary content, finances, location, calendar) in any output. Output answers in your own words, framed as decisions on the question at hand." This line is load-bearing and must not be edited away.

### Persona substrate scope

`research.md` declares `## Context` hints for `private-source/telos/MISSION.md`, `private-source/telos/GOALS.md`, `private-source/telos/PROJECTS.md`, `private-source/telos/BELIEFS.md`, all of `private-source/memory/*.md` (48 files, ~120K chars). No filtering; the persona-agent decides at runtime which substrate is relevant per question. Total token cost ~31K, fully cacheable across runs.

### Idempotency contract

Three markers, each a single HTML comment line:

- PRD comment: first line is `<!-- minion:research run-id=<sha7-of-issue-events-or-similar> -->`.
- Child issue body: first line is `<!-- minion:slice parent=#<N> slice=<n>/<total> -->`.
- Slicer's parent-issue status comment: first line is `<!-- minion:research-status parent=#<N> -->`.

The publisher agent's prompt instructs it to query the parent issue's existing comments via `gh issue view --json comments` and skip writing comments whose marker matches an existing one (regenerating instead).

### Build trigger semantics (revised — see banner)

- `minion-research` label OR `/minion research` comment on parent issue → fires `research.yml`, which posts the PRD + slice-plan comments and stops.
- Implementation is **not** auto-cascaded. When jcleira is ready he labels the **parent** `minion-approved` (or comments `/minion build`); `minion.yml` fires `implement.md` once on the parent → one feature PR.
- Each build gets a unique branch `minion/<prog>-<agent>-<issue>` (minions ≥ v0.0.6), so re-runs and separate features never share a branch/PR.
- (Originally: the slicer created child issues each pre-labeled `minion-approved`, auto-firing `implement.md` per child. Replaced — it proliferated issues and collided PRs on a shared branch.)

### Architectural decisions

- **Clone the private source repo fresh per workflow run** (vs symlinking the runner-resident clone, vs snapshotting from runner filesystem). Reproducibility (workflow log shows exact private-source SHA used) and portability (no runner-host coupling) outweigh the ~5s clone overhead. Alternative paths were considered and rejected; revisit if the laptop runner is replaced or augmented.
- **Each minion phase runs as its own Claude session within a single program run** (sub-agents in `## Agents`), not as separate program files chained through GH. Within-run sub-agents are simpler to coordinate, share a worktree natively, and produce one workflow run per parent issue (cleaner observability) instead of N runs across the chain.
- **All artifacts on GitHub** (issues, comments, PRs); no `docs/` files committed. Keeps the chain on a single surface; `gh` is already available in the agent's Bash tool; no new sync mechanism between docs/ and the issue tracker.
- **Three labels not one.** `minion-research`, `minion-research-output`, `minion-research-completed` separate input trigger, child provenance, and parent state. Each is queryable independently.
- **No PR created by the research run**. The research minion produces only GitHub side effects (comments, issue creates, label adds). The existing executor handles "no worktree changes" by marking the run skipped (not failed); the run still succeeds and side effects persist.

### Schema changes

Three new repo labels on `partio-io/cli`. No data schema, no API contracts, no migrations.

### API contracts

No new APIs. All inter-component communication is one of: (a) GitHub label/issue/comment state, (b) files in the worktree shared between sub-agents, (c) idempotency markers in comment/issue body content.

### Specific interactions

- Human applies `minion-research` to parent issue → `research.yml` fires.
- Workflow clones the private source repo into `${{ github.workspace }}/private-source/`.
- Workflow runs `minions run .minions/programs/research.md --issue <N>`.
- Sub-agents run sequentially: researcher → persona → prd-writer → slicer → publisher.
- Publisher posts PRD comment, opens child issues, labels parent.
- Workflow ends without closing parent.
- Each child issue's `minion-approved` label causes `minion.yml` to fire on it.
- `implement.md` runs per child, opens PR.
- Human reviews + merges each PR.
- Human manually closes parent issue when satisfied.

## Testing Decisions

- **What makes a good test for this system**: end-to-end runs against real test issues. The system's behavior is dominated by Claude prompts, GitHub side effects, and workflow YAML semantics. Unit tests on agent prompts have low signal because the prompts are natural-language; small text changes don't break parseability and large semantic changes are not caught by any cheap assertion.
- **External behavior (what to test)**: the contract is "given a complex parent issue with `minion-research` label, the system produces a PRD comment + N child issues + parent labeled `minion-research-completed`, and child PRs eventually appear via the existing implement chain." Anything internal to the program (agent ordering, transcript file format, prompt phrasing) is implementation detail.
- **Implementation details (what NOT to test)**: transcript file format, sub-agent prompt phrasing, the specific token counts of substrate, the exact wording of the persona privacy directive (its presence is what matters; its phrasing is tuned over time).

### Modules to verify

- **`research.md` parses cleanly**: covered by partio-minions's existing `internal/program/parse_test.go` infrastructure; running `minions run research.md --dry-run` is sufficient to validate the program file. No new test code required.
- **`research.yml` parses cleanly**: GitHub validates workflow YAML on push; failures surface immediately. No new test code required.
- **End-to-end smoke**: a controlled first run on a small/known parent issue, with the slicer's `minion-approved` label removed manually before children fire, so the research output is reviewed before any implement runs. After the first successful smoke run, future runs trust the cascade.

### Modules to consider for future tests (post-v1)

- **Idempotency**: integration test that runs `research.md` twice on the same parent issue and verifies no duplicate PRD comment, no duplicate child issues. Build only if a real re-run produces duplicates.
- **Persona privacy**: a regression assertion scanning research run logs (or output artifacts) for known-personal-data substrings (e.g., specific Whoop / TrainingPeaks / financial terms). Build only if a leak is observed in a real run.

### Prior art

- `partio-minions/internal/program/parse_test.go` validates program parsing. Continues to apply.
- No existing test for workflow behavior or for cross-program label-chained execution. This feature does not introduce new tests there; smoke runs are the verification.

## Out of Scope

- **Persona "tune-up gotchas" file**. Defer until persona misbehavior is observed in real runs; predicting gotchas without data wastes effort.
- **Pre-flight check that private-source clone is on master and clean**. The clone-fresh strategy sidesteps mid-state risk.
- **Automated parent-close when all child PRs are merged**. Manual close is fine for v1.
- **Escalation logic in `implement.md`** ("this task is too big, escalate to research"). Trust human triage to choose path.
- **Cost / quota dashboards**. Subscription quota is the real ceiling; existing run logs report token counts. Build observability only if quota becomes a constraint.
- **Multi-runner / hosted-runner support beyond the existing self-hosted laptop runner**. Workflow clones the private source repo via PAT to keep the future open, but no hosted runner is being provisioned now.
- **PRD-comment versioning / history**. First version overwrites or regenerates; no diff tracking.
- **Persona "learning" from past runs**. Persona is stateless per run. Tuning happens via the gotchas file (out of scope) or via TELOS/memory updates (continuous, separate process).
- **Changes to existing simple-task flow**. `minion.yml`, `implement.md`, `propose.md`, `propose.yml`, `approve.md`, `doc-update.md`, `ingest.md`, `readme-update.md` all unchanged.
- **Replacement or refocus of propose pipeline**. Decision was to keep it running unchanged.
- **README rewrites for partio-minions or cli**. Acknowledged as stale; not in this PRD.
- **`approve.yml` cron** (auto-approve proposals after 24h). Mentioned as a missing piece elsewhere; not part of this PRD.
- **Author_association tightening on existing `minion.yml`**. Worth doing eventually; only `research.yml` requires it for this feature.
- **Minion-failure recovery beyond the existing `minion-failed` label + log link comment**. Same observability as today.

## Further Notes

- The self-hosted runner `github-runner-partio-minion-ai-01` is on jcleira's laptop (`/home/arvos/...`). OAuth subscription auth flows through `~/.claude/.credentials.json` on that machine. Any infra change affecting the laptop affects the runner.
- Subscription quota is the real cost ceiling. API-equivalent valuations in run logs (~$0.83/propose-run) are not billed dollars under OAuth; usage counts against Claude Max rolling/weekly caps. Many one-shots per day may bump those caps.
- the private source repo's default branch is `master`, not `main`. Workflow clone step must respect that.
- This design is the spiritual sibling of the `/code-research` → `/code-create-prd` → `/code-create-issues` → `/code-execute-issue` skill chain, executed headlessly with a persona-agent in jcleira's seat and GitHub artifacts replacing local doc files.
- partio-minions's reboot history (partio-minions-claude-code → minion-multirepository → minion-programs → minion-programs-only → current) deliberately stripped functionality. This PRD is purely additive at the program/workflow/config layer; it does not reverse the simplification trajectory of the binary.
- The current propose pipeline produces 1–5 `minion-proposal` issues/day from competitor monitoring of `entireio`. Some of those may be the natural first inputs for the research minion: triage proposal → judge complexity → label `minion-approved` (simple) or `minion-research` (complex).
- This PRD lives in `partio-minions/docs/` because the research-minion design is fundamentally about the minion runtime's usage pattern. The implementation artifacts (`research.md`, `research.yml`, labels, PAT scope) all land in `partio-io/cli` when built.
