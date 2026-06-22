# 08 — Persona substrate: in-repo telos+memory

**Blocks**: [#6 production smoke](./06-production-smoke-run.md) — do not run research on substantive issues until this lands

> **This is a handoff.** It records a decision and the work to execute it.
> A fresh `/code-execute-issue` session can pick it up.

## Decision (why this exists)

The `persona` agent must be grounded **only** by a sanitized, in-repo
`telos` + `memory` substrate committed to `partio-io/cli` under
`.minions/persona/`. That substrate lives in a public repo, so it must
contain **no personal data** — only generic, publishable decision-making
principles, product/engineering context, and preferences. The pipeline
must not source the substrate from anywhere outside the repo at runtime.

## Status

- [x] The persona no longer depends on any out-of-repo substrate source.
- [ ] **jcleira** revokes the underlying PAT in GitHub account settings
  (Settings → Developer settings → Personal access tokens). This is the
  definitive step — deleting the CI secret only removed the copy.

## What to build

1. **Create the in-repo substrate** in `partio-io/cli`:
   `.minions/persona/telos/*.md` + `.minions/persona/memory/*.md`,
   co-located with the program that consumes it. Hand-authored and
   reviewed; **sanitized for a public repo** — no personal data, and no
   content copied verbatim from elsewhere. Seed it with the
   decision-relevant essence: how jcleira weighs engineering trade-offs,
   partio product priorities, quality bar, and defer-domains — written as
   publishable guidance.

2. **Point the `persona` agent in `research.md`** at the in-repo
   substrate via repo-relative paths: a `## Context` section listing the
   four TELOS files, plus a runtime read of `.minions/persona/memory/`.
   Keep the privacy directive.

3. **Confirm `research.yml`** needs no secret beyond `GH_PAT`.

4. **(Future, optional, local-only)** Document a manual refresh process:
   a human curates the in-repo substrate locally from their own notes,
   reviewing the diff for leaks before commit. Never automate sourcing it
   from outside the repo.

## Acceptance criteria

- [x] An in-repo `telos` + `memory` substrate exists in `partio-io/cli`
  and contains no personal data (privacy review of the committed files).
- [x] `research.md`'s `persona` reads only the in-repo substrate.
- [x] `research.yml` needs no secret beyond `GH_PAT`.
- [ ] A research run on a test issue produces a substantive PRD grounded
  in the in-repo substrate.
- [ ] Privacy review of that run's public comments: no personal data, no
  verbatim substrate dumps.
- [x] The persona privacy directive is still present in the prompt.

## Modules touched

- `partio-io/cli/.minions/persona/` (telos + memory files).
- `partio-io/cli/.minions/programs/research.md` (`persona` agent + `## Context`).
- `partio-io/cli/.github/workflows/research.yml`.

## Out of scope

- The production smoke (#6) — it depends on this and runs after.
- Re-run idempotency (#5).
