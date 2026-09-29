# 06 — Publish the runtime release

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [03 — Fail and repair an abandoned declaration](./03-fail-and-repair-abandoned-declaration.md), [04 — Exempt a declaration a later slice will use](./04-exempt-declaration-used-later.md), [05 — Catch a slice that deletes an earlier slice file](./05-catch-deleted-earlier-slice-file.md)

## What to build

The guard runs in the partio-minions runtime, and the cli workflows
install that runtime at a pinned version. Until a new version exists,
none of issues 01 to 05 changes a single build.

This slice prepares the release and hands the publish step to the
operator. It does not publish anything itself: a release is an external
side effect, so it needs a human trigger.

Prepare:

- Confirm the default branch carries issues 01 to 05 and that the suite
  is green on it.
- Draft the release notes.
- State the next version number.

The notes must say that this tag also carries work already on the branch
but never released: the working copy sits one commit past `v0.0.13`, on
`argos/resume-marker-range`, with a fix that counts slice markers on the
branch rather than on its base. Anyone reading the release needs to know
that fix ships here too.

There is no release tooling. The Makefile builds, tests and lints, but
never creates a tag. The version reaches the binary from `git describe`,
so publishing a GitHub Release at the merge commit is what creates the
tag and sets the version.

Record one known failure mode in the notes: `go install` can fail
against a minutes-old tag because the checksum database has not ingested
it yet. The first run after a release can therefore fail for a reason
unrelated to this work. Retry rather than debug.

## User stories covered

PRD story 23.

## Acceptance criteria

- [x] The default branch carries issues 01 to 05 and the full suite is
      green on it.
- [x] A release notes draft exists and names the guard behavior in terms
      an operator can check.
- [x] The notes state that this tag also carries the previously
      unreleased slice-marker counting fix.
- [x] The notes state the checksum-database lag and that a first failed
      install should be retried.
- [x] The next version number is stated explicitly.
- [x] Nothing is tagged, published or pushed by this slice.
- [x] The handoff block below is posted to the operator before this
      slice's row is flipped in `issues.md`.

## Modules touched

None. This slice produces release notes and a version decision, not
code.

## Test prior art

None — there is no code change. The gate is the existing suite on the
default branch.

## Out of scope

- Publishing the release. The operator does it, per the handoff.
- Bumping the version pin in the cli workflows. Issue 07.
- Adding release tooling or a Makefile tag target. Neither is needed to
  ship this feature, and adding one is a separate decision.

## Handoff

When this slice's acceptance criteria all pass, BEFORE flipping its row
in `issues.md`, post the following block to the user verbatim:

- **URL / artefact to visit**:
  `https://github.com/partio-io/minions/releases/new` — publish a
  Release at the default branch's merge commit, using the drafted notes.
- **Action required**: choose the version number and publish the
  Release. Publishing creates the tag; nothing else does.
- **Where to record the decision**:
  - In `issues.md` (add a one-line note next to this slice's row), AND
  - In issue 07's file under the `Chosen version:` field
    (e.g. `Chosen version: v0.0.14`)

Issue 07 MUST NOT start until that field is filled.
