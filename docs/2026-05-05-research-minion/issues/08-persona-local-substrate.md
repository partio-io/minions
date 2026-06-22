# 08 — Persona substrate: in-repo telos+memory, drop the external repo entirely

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: nothing (the external clone has already been cut — see "Status" below)
**Blocks**: [#6 production smoke](./06-production-smoke-run.md) — do not run research on substantive issues until this lands

> **This is a handoff.** It records a decision and the work to execute it.
> A fresh `/code-execute-issue` session (or jcleira) can pick it up.

## Decision (why this exists)

The `persona` agent was grounded in an **external repo** (`telos/*.md`
+ `memory/*.md`) that `research.yml` cloned into the **public**
`partio-io/cli` at runtime. The persona then posts PRD + slice-plan
comments on public issues. Pulling external, unreviewed content into a
public pipeline that writes world-readable comments is a risk: the only
safeguard between that content and a public comment was an LLM prompt
directive.

**Decision: the minion pipeline must never touch the external repo.** Instead, lock
a curated, **sanitized, in-repo** `telos` + `memory` into the minion
repo and use *only* that as the persona substrate. The substrate lives
in a public repo, so it must contain **no personal data** — only
generic decision-making principles, product/engineering context, and
preferences safe to publish. The external repo may be used to *inform* that
substrate **manually, locally**, by a human producing a sanitized diff
— never by a runtime clone.

## Status — the external repo access already severed (security, done first)

Done ahead of this slice because it is security-sensitive:

- [x] `research.yml`: "Clone the external repo" + "Print external SHA" steps and the
  `secrets.PERSONA_SUBSTRATE_PAT` reference removed (partio-io/cli#454).
- [x] `PERSONA_SUBSTRATE_PAT` repo secret deleted from `partio-io/cli`.
- [ ] **jcleira** revokes the underlying PAT in GitHub account settings
  (Settings → Developer settings → Personal access tokens). This is the
  definitive step — deleting the secret only removed the CI copy.

Consequence: until this slice ships, the `persona` runs with **no
substrate** (it can no longer find the external repo). Research output will be
generic. **Do not** trust a research run on a substantive issue until
the in-repo substrate exists.

## What to build

1. **Create the in-repo substrate** in `partio-io/cli` (recommended:
   `.minions/persona/telos/*.md` + `.minions/persona/memory/*.md`,
   co-located with the program that consumes it). Hand-authored and
   reviewed; **sanitized for a public repo** — no personal data, and no
   content copied verbatim from external sources. Seed it with the
   decision-relevant essence:
   how jcleira weighs engineering trade-offs, product priorities for the
   magik-family/partio, quality bar, defer-domains, etc., written as
   publishable guidance.

2. **Rewrite the `persona` agent in `research.md`** to read **only** the
   in-repo substrate via repo-relative paths. Remove the external-clone
   resolution logic and the `## Context` entries that
   point at `external/telos/*.md` / `external/memory/*.md`; point them at the
   new `.minions/persona/...` paths. Keep the privacy directive (defense
   in depth, even though the substrate is now public-safe).

3. **Confirm `research.yml`** has no external clone step (done in #454) and that
   the run needs no secret beyond `GH_PAT`.

4. **(Future, optional, local-only)** Document a manual refresh process:
   a human reads the external repo locally and updates the sanitized in-repo
   substrate, reviewing the diff for leaks before commit. Never automate
   a clone of the external repo into these repos.

## Acceptance criteria

- [x] `research.yml` does not clone the external repo and references no `PERSONA_SUBSTRATE_PAT`
  (verify; shipped in #454).
- [x] An in-repo `telos` + `memory` substrate exists in `partio-io/cli`
  and contains no personal data (privacy review of the committed files).
- [x] `research.md`'s `persona` reads only the in-repo substrate; no
  external path or clone reference remains anywhere in `research.md`.
- [ ] A research run on a test issue with **no network access to the external repo**
  still produces a substantive PRD grounded in the in-repo substrate.
- [ ] Privacy review of that run's public comments: no personal data,
  no verbatim substrate dumps.
- [x] The persona privacy directive is still present in the prompt.

## Modules touched

- `partio-io/cli/.minions/persona/` (new — telos + memory files).
- `partio-io/cli/.minions/programs/research.md` (`persona` agent +
  `## Context`).
- `partio-io/cli/.github/workflows/research.yml` (clone already removed
  in #454).

## Test prior art

- `03-prd-writer-comment-label.md` / `07-slice-plan-comment-manual-build.md`
  — the persona/substrate flow this revises.
- `partio-cli/.minions/programs/research.md` — current `persona` prompt
  (the external resolution logic to remove).

## Out of scope

- Any automated external→minion sync. Refresh is manual and local only.
- The production smoke (#6) — it depends on this and runs after.
- Re-run idempotency (#5).
