# 07 — Bump the minions pin in the cli workflows

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: [06 — Publish the runtime release](./06-publish-runtime-release.md)

**Chosen version**: `v0.0.14`

## What to build

The last step that makes the guard real. The cli workflows install the
minions runtime at a pinned version, so a published release changes
nothing until every workflow points at it.

The pin is copy-pasted. There is no shared composite action and no
reusable workflow, so the same install line appears in each workflow
file that runs minions. Every one must move together: a build workflow
left on the old pin silently keeps the old runtime, and the guard simply
does not run on those builds.

This slice edits only the cli repository. It opens a pull request there
and stops. The operator merges.

Do not start until the `Chosen version` field above is filled.

## User stories covered

PRD stories 23, 24.

## Acceptance criteria

- [ ] The `Chosen version` field above is filled before any edit.
- [ ] Every workflow file in the cli repository that installs the
      minions runtime pins the chosen version.
- [ ] No workflow file is left on the previous pin — verified by
      searching the workflows directory for the old version string and
      finding no match.
- [ ] The chosen version resolves: installing it succeeds, allowing for
      a retry if the checksum database has not yet ingested the tag.
- [ ] A pull request is opened against the cli repository with these
      edits and nothing else.
- [ ] The pull request is not merged.

## Modules touched

None in partio-minions. The change is in the cli repository's workflow
definitions.

## Test prior art

None — these are workflow definitions, not Go code. The proof is the
search for the old pin returning no match, and the first minion run on
the new pin completing.

## Out of scope

- Merging the pull request. The operator merges.
- Replacing the copy-pasted pin with a shared composite action. It would
  remove this whole class of work, but it is a separate change and a
  separate decision.
- Enabling slice builds in any other repository.
