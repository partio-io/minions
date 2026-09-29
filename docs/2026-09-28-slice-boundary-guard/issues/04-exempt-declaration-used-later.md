# 04 — Exempt a declaration a later slice will use

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [03 — Fail and repair an abandoned declaration](./03-fail-and-repair-abandoned-declaration.md)

## What to build

A normal slice plan stops tripping the guard.

A common and correct plan shape adds a primitive early and consumes it
later. Slice 1 adds a fetch helper, and slice 3 is the slice that calls
it. Between them, slice 2 finishes with that helper unreferenced — which
is exactly what issue 03 fails on.

Without this slice, that plan fails on slice 2 every time, burns a fix
session, and either rewrites the plan's intent or stops the run. The
guard would punish good plans.

Exempt a contribution while any slice after the current one names its
identifier in the plan text. The exemption lifts once no later slice
remains to consume it, so the final slice still catches genuine
abandonment.

The lookup belongs in `internal/slices`, next to the parser that already
owns plan knowledge. The guard asks the plan; it does not parse plan
text itself.

Note that the plan is prose written by a research agent, so the match is
on the identifier appearing in a later slice's text, not on a structured
field.

## User stories covered

PRD story 12.

## Acceptance criteria

- [x] `internal/slices` answers whether any slice after a given number
      names a given identifier in its plan text.
- [x] The guard treats a contribution as exempt while such a later slice
      exists, and reports no finding for it.
- [x] The exemption lifts on the last slice, so a contribution no
      remaining slice consumes is still reported.
- [x] A plan of the shape "slice 1 adds a primitive, slice 3 consumes
      it" passes the guard on slice 2 without a fix session.
- [x] The same plan still reports a finding when slice 3 finishes
      without consuming the primitive.
- [x] An identifier that appears only in an earlier slice's text, or
      only in the current slice's text, grants no exemption.
- [x] The guard does not parse plan text itself; it asks the slices
      package.

## Modules touched

- `internal/slices` — the exemption lookup beside the plan parser.
- `internal/sliceguard` — applying the exemption before reporting.

## Test prior art

The slices package's parser tests are table-driven over plan comment
bodies and are the direct model for the lookup's cases. The exemption's
effect on findings belongs at the `sliceguard` seam, beside the
detection cases added in issue 03.

## Out of scope

- Any change to how plans are written or parsed into slices.
- Deleted files. Issue 05.
