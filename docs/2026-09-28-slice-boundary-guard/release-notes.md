# Release notes draft — slice boundary guard

**Next version**: `v0.0.14`. The newest tag is `v0.0.13`, and every tag
so far is a `v0.0.x` step.

**Target**: `57e5313` on `main`, the merge commit of PR #145. It holds
all the code of this release. A later `main` commit that adds only docs
builds the same binary.

**How to publish**: create a GitHub Release at the target, with the body
below. The Release creates the tag. Nothing else does: the Makefile
never tags, and it stamps the version from `git describe`.

## Body for the Release

Adds a slice boundary guard to slice builds. A later slice can no longer
silently redo or undo the work of an earlier slice in the same run.

- **Prompt.** The prompt of slice two and later has an `## Already Built`
  section. It lists each package-level Go declaration that an earlier
  slice of the run added, with its file and its slice. The prompt of
  slice one has no such section.
- **Abandoned code.** After the session of a slice ends, and before its
  work is committed, the guard reads the working tree. The checks of the
  slice fail when nothing references a package-level Go declaration that
  an earlier slice added. A reference from a test file does not count.
- **Deleted files.** The checks also fail when a file that an earlier
  slice added is no longer at its path. This applies to a file of any
  kind, and a rename counts as a deletion.
- **Repair.** The guard is part of the checks of a slice, so it runs only
  for an agent with `checks: true`. With `retry_on_fail: true`, a guard
  failure gets one fix session, then one more check. The failure text
  names each identifier or file, the slice that added it, and the
  repair: call the earlier code and remove the duplicate, or restore the
  file and use it.
- **Signals.** Each finding logs a warning that starts with
  `slice boundary guard:`. A repaired slice continues with no pull
  request line and no issue comment. An unrepaired slice fails. The run
  stops at that slice, the earlier slices stay pushed, and the issue
  gets the usual resume comment.
- **Exemptions.** A declaration is exempt while the plan text of a later
  slice names it. On the last slice, no declaration is exempt. Slice one
  never fails on the guard. Code that the base branch carried is never
  judged.
- **Limits.** The declaration check reads Go only. If a Go file does not
  parse, the declaration check reports nothing for that repository, and
  leaves the broken file to the other checks. A repository without Go
  gets no declaration finding, but the deletion check still applies to
  it. In a multi-repo build, the guard scans each repository alone, so a
  declaration that only a different repository uses counts as
  abandoned. Neither limit applies to the cli `implement` program, which
  builds one Go repository.
- **Dry run.** In a `--dry-run` of a new task, the `## Already Built`
  section says that no declarations were found, because no slice has
  run. The run also logs `slice contributions: cannot read the branch,
  skipping` once per repository for each slice after slice one. The
  warning is harmless.
- **Configuration.** There is no new configuration key and no new module
  dependency.

This tag also carries a fix that no earlier tag holds: the runtime now
counts slice markers on the branch, not on its base. Before the fix,
resume failed in each repository that had merged minion work before,
with the error `branch has N slice markers but the plan has M slices`.

Known failure on the first install: `go install` can fail against a
minutes-old tag, because the Go checksum database has not ingested the
tag yet. The first minion run after this release can fail for this
reason, which is not related to this work. Retry after a few minutes.
Do not debug it.

See #145.
