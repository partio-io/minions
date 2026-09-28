# 02 — List earlier slice contributions in each slice prompt

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately

## What to build

A slice session can see what the earlier slices in the same run already
built, without a search.

Today a slice's prompt carries the issue title and body, the PRD
comment, the whole slice plan for orientation, and the slice's own
assignment. It carries no record of what already exists in the tree. A
slice therefore has to discover earlier work by accident, and when it
does not, it rebuilds that work.

Add an "already built" section to the prompt for slice two and later.
It lists, for each package-level Go declaration that an earlier slice in
this run added: the identifier, the file that declares it, and the slice
number that added it.

Slice one has no earlier slices, so its prompt omits the section
entirely rather than showing an empty heading.

This slice creates the shared substrate that issue 03 also needs: the
git reads that list a commit range's commits and changed files, and the
analysis that partitions the branch's commits on the empty slice marker
commits and extracts the declarations each slice added.

Only commits on the run's own branch are partitioned. Code the base
branch already carried is never attributed to a slice.

Verify it with a dry run: the per-slice prompts print without a Claude
session.

## User stories covered

PRD stories 1, 11, 18, 21, 22, 26.

## Acceptance criteria

- [ ] The git package can list the commits of a range and the files each
      commit changed, alongside the existing subject-listing helper.
- [ ] A new `internal/sliceguard` package reports the contributions of
      the slices before a given slice number, for a given repository.
- [ ] A contribution names the declared identifier, the declaring file,
      and the slice number that added it.
- [ ] Contributions cover package-level functions, types, constants and
      variables, whether exported or not.
- [ ] Slice attribution uses the existing empty marker commits, so slice
      identity keeps one definition in the runtime.
- [ ] Declarations that the base branch already carried are not reported
      as any slice's contribution.
- [ ] The prompt for slice two and later carries an "already built"
      section listing those contributions.
- [ ] The prompt for slice one omits the section entirely.
- [ ] A repository the analysis cannot parse yields no contributions and
      no error that stops the run.
- [ ] The runtime gains no new module dependency; only the standard
      library is used for the parsing.
- [ ] A dry run prints the per-slice prompts including the new section.

## Modules touched

- `internal/git` — per-range commit and changed-file reads.
- `internal/sliceguard` — new package; the contribution report.
- `internal/executor` — the per-slice prompt builder.

## Test prior art

The git package builds a temporary repository per case with a local
helper and asserts against real git output; the range-listing tests are
the closest analogue for the new reads. The executor package's
slice test file already asserts per-slice prompt content through a dry
run — extend that seam rather than adding a new one. The slices package
shows the table-driven style used across the repo.

## Out of scope

- Detecting that a contribution has been abandoned. Issue 03 does that.
- Any change to the check path or the retry. Issue 03 does that.
- Exemption of a contribution a later slice will consume. Issue 04.
- Deleted files. Issue 05.
