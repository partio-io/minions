# PRD — Slice boundary guard

**Date**: 2026-09-28. **Repo**: `partio-io/minions`.

## Problem Statement

jcleira sends an issue to the minion pipeline. The pipeline splits the
issue into slices and builds one slice per Claude session. Each session
gets a fresh worktree that carries the earlier slices' commits.

A later slice can rebuild what an earlier slice already built. The
earlier slice's code then has no caller. A separate dead-code audit
deletes it after the pull request opens. The run stays green, and
nothing tells jcleira that the pipeline paid for work it then threw
away.

This is not a theory. Run `36099180129` built issue #31 in four slices:

1. Slice 1 added a `FetchBranch` git primitive with tests. Its checks
   passed. It cost 3 minutes and 38 seconds and 163 lines.
2. Slice 2 implemented `Reconcile()` and wrote the same fetch inline
   instead of a call to the new primitive.
3. `FetchBranch` then had no caller.
4. The dead-code audit deleted the primitive and its test.

The four slice commits added 780 lines. Pull request #727 shows 617.
The 163-line gap is slice 1. Every check was green at every step.

Two properties make this invisible today:

- The per-slice checks cannot catch it. Slice 1's code was correct and
  its tests passed. The waste only exists once slice 2 lands.
- The audit runs in a different workflow, about seven minutes after the
  build run ends. The build run had already exited green. It could not
  report the loss even in principle.

The rule that forbids this already exists. The `implement` program tells
each slice that earlier slices are committed on its branch, and to build
on their work and never redo or undo it. Slice 2 ignored it. So a second
instruction is the remedy that already failed here.

## Solution

The pipeline gets two changes that work together.

**Prevention.** Each slice's prompt gains a section that lists what the
earlier slices already built — the symbol, the file that declares it,
and the slice that added it. A slice no longer has to discover the
earlier work by accident. Today the prompt carries the issue, the PRD,
the whole slice plan, and the slice's own assignment, but no record of
what already exists in the tree.

**Repair.** After each slice's session, and before the slice is
committed, a deterministic guard reads the working tree. The guard fails
the check when either of these holds:

- An earlier slice added a package-level declaration, and nothing in the
  tree references it any more.
- The current slice deleted a file that an earlier slice added.

A failed check already has a defined path: the executor runs exactly one
fix-it session with the failure text as its prompt, then re-runs the
checks. The guard reuses that path. Its failure text names the abandoned
symbol, the file, the slice that added it, and the required repair — call
the earlier code and drop the duplicate.

The repair direction is fixed on purpose. The current slice reuses the
earlier slice's work. It does not delete it. Deletion is the outcome the
audit already produces today, and it is the outcome this PRD exists to
prevent.

If the fix session cannot repair the tree, the slice fails and the run
stops at that slice. This matches every other failed check. The earlier
slices stay pushed, and the issue gets the usual resume comment.

A successful repair is silent. A reuse repair loses no work, so there is
nothing for jcleira to read.

## User Stories

1. As jcleira, I want a slice to know what the earlier slices already
   built, so that it calls that code instead of writing it again.
2. As jcleira, I want the pipeline to detect a slice that abandons
   earlier work, so that the waste never reaches a pull request.
3. As jcleira, I want the detection to run one slice after the damage,
   so that the repair is small and local.
4. As jcleira, I want a slice to repair itself and continue, so that a
   single slicing mistake does not cost me a whole run.
5. As jcleira, I want the run to fail when the repair fails, so that a
   broken slice boundary never opens a green pull request.
6. As jcleira, I want the repair to reuse the earlier slice's code, so
   that the slice plan I approved still describes the tree I get.
7. As jcleira, I want the repair to never delete an earlier slice's
   work, so that the pipeline stops paying for code it throws away.
8. As jcleira, I want a successful repair to stay silent, so that a
   green run means the same thing it always did.
9. As jcleira, I want the guard to catch a slice that deletes a file an
   earlier slice added, so that "never undo" is enforced and not just
   requested.
10. As jcleira, I want the guard to be a deterministic program and not a
    Claude session, so that the same tree always gives the same verdict.
11. As jcleira, I want the guard to add no new dependency, so that the
    runtime keeps its small dependency set.
12. As jcleira, I want a helper that a later slice will legitimately use
    to pass the guard, so that a normal "add a primitive first" plan
    does not fail on every slice.
13. As jcleira, I want the guard to read the working tree and not only
    the commits, so that it sees the current slice's work before that
    work is committed.
14. As jcleira, I want the guard to run for every repo in a multi-repo
    build, so that the boundary holds wherever the slice wrote code.
15. As jcleira, I want the guard to skip a repo whose language it cannot
    analyze, so that a non-Go repo is never failed by a check that
    cannot judge it.
16. As jcleira, I want the failure text to name the symbol, its file and
    its slice, so that the fix session can act without a search.
17. As jcleira, I want the failure text to state the required repair
    direction, so that the fix session reuses rather than deletes.
18. As a slice session, I want the list of earlier contributions in my
    prompt, so that I can call existing code on my first attempt.
19. As a slice session, I want one fix attempt with a precise failure
    message, so that I can repair the boundary without a second guess.
20. As jcleira, I want the guard to reuse the existing retry budget, so
    that no new failure route or new configuration appears.
21. As jcleira, I want the first slice to never be blocked by the guard,
    so that a run with nothing before it starts normally.
22. As jcleira, I want the guard to ignore code that the issue's base
    branch already carried, so that only the run's own slices are
    judged.
23. As jcleira, I want the change to ship in the runtime, so that every
    repo that builds in slices inherits the guard.
24. As jcleira, I want the PRD to record the deploy cost, so that I know
    a runtime release needs the version pin bumped in each cli workflow.
25. As jcleira, I want the guard covered by the existing live slice
    tests, so that its behavior is proven through the real slice loop
    and not only through a unit.
26. As jcleira, I want the prompt change covered by the existing dry-run
    test, so that I can see the prompt a slice receives without a
    Claude session.
27. As jcleira, I want a run that never trips the guard to behave
    exactly as it does today, so that the change carries no cost on a
    clean build.

## Implementation Decisions

**Language.** Go. The repo is already Go, so the repo wins and the
language policy needs no further decision. The guard uses `go/ast`,
`go/parser` and `go/token` from the standard library, so the runtime
adds no dependency.

**New module — `internal/sliceguard`.** One deep module behind a small
surface. It answers two questions and owns every mechanism behind them:

- What did the earlier slices contribute? It returns a contribution per
  package-level declaration, with the declaring file and the slice
  number that added it.
- Which of those contributions does the current working tree abandon?
  It returns a finding per abandoned contribution and per deleted file.

Everything else stays unexported: the commit partition, the symbol
extraction, the reference scan, and the exemption rule. The two entry
points are the only surface, and both halves of the feature — the prompt
and the check — call the same module. This is the reason `sliceguard` is
a package and not a helper inside the executor.

**Slice attribution.** The runtime already writes an empty marker commit
after each slice. The guard partitions the branch's commits on those
markers to decide which slice added which declaration. This reuses the
marker convention that the resume logic already depends on, so slice
identity has one definition in the runtime, not two.

**The working tree is the source of truth.** The per-slice check runs
before the current slice's work is committed. So the guard reads the
declarations of earlier slices from commits, and judges references
against the files on disk. This is what lets the guard see a duplicate
that the current session has written but not yet committed.

**Scope of a contribution.** Package-level declarations — functions,
types, constants and variables — whether exported or not. An abandoned
unexported helper wastes the same build time as an exported one.

**Reference scan.** The guard parses the repo's Go files and looks for
uses of each contributed identifier outside its own declaration. A
declaration with no remaining use is abandoned. A test file counts as a
use only when the declaration is used outside tests as well, because a
symbol that only its own test calls is exactly the dead code this
feature exists to catch.

**Exemption by plan.** A contribution is exempt while any slice after
the current one names its identifier in the plan text. This keeps the
common shape — slice 1 adds a primitive, slice 3 consumes it — from
failing on every intermediate slice. The lookup lives in
`internal/slices`, next to the parser that already owns plan knowledge.

**Deletion check.** The guard flags the current slice when it removes a
file that an earlier slice added. The data is already in hand from the
commit partition, so this case costs almost nothing to cover.

**Integration point.** The guard runs inside the per-slice check path,
so its result joins the existing check output. This is deliberate: the
executor already runs one fix-it session on a failed check, with the
failure text as the prompt, and already fails the slice when the second
pass fails. The guard therefore needs no new failure route, no new
retry budget and no new configuration key.

**No new plumbing for context.** The fix session's prompt today is the
captured check output and nothing else. Rather than thread the slice
plan into that session, the guard writes everything the session needs
into its own failure text. This keeps the change inside the guard and
leaves the executor's retry contract untouched.

**Prompt change.** The slice prompt builder gains a section that lists
the earlier contributions. The section sits with the existing context
the prompt already carries.

**Language guard.** The guard reports nothing for a repo it cannot
analyze. A non-Go repo therefore behaves exactly as it does today.

**First slice.** Slice one has no earlier contributions, so the guard
returns nothing and the prompt section is omitted.

**Base branch.** Only commits on the run's own branch are partitioned.
Code that the base branch already carried is never attributed to a
slice and is never judged.

## Testing Decisions

A good test states external behavior. It calls a public boundary, gives
real inputs, and asserts the result a user would notice. It does not
assert the shape of an internal call, a private field, or the order of
unexported helpers. Each test in this feature must survive a rewrite of
the guard's internals.

Every module named in this PRD is tested. No module is exempt.

**Seams.** Three, of which two already exist.

1. `executor.Run`, through the live slice-loop tests *(existing seam)*.
   This is the highest seam available and the primary one. It drives a
   real git repo through a real slice loop. It already proves the retry
   path this feature extends, so the new behavior belongs beside the
   existing retry cases rather than in a new harness. It covers: a slice
   that abandons earlier work trips the guard, the fix session repairs
   it, and the run continues to a pull request; and the fail case, where
   the repair does not hold, the run stops at that slice, and the
   earlier slices stay pushed.
2. `executor.Run` in dry-run mode, through the existing per-slice prompt
   test *(existing seam)*. It already asserts prompt content, so the
   prevention half needs no new seam. It covers: the prompt for slice
   two and later carries the earlier contributions, and the prompt for
   slice one does not.
3. `sliceguard.Inspect` *(new seam)*. This is the one new seam, and it
   is justified by cost. The detection matrix — exported and unexported
   declarations, a symbol used only by its own test, exemption by a
   later slice's plan text, a deleted file, a multi-repo build, a
   non-Go repo, and the first slice — is impractical to drive through a
   full slice loop, and each case needs a purpose-built repository. The
   seam sits at the package's public entry point, not at its internals,
   so the mechanism stays free to change.

No lower seam is added. The commit partition, the symbol extraction and
the reference scan are covered through `Inspect`, because they are the
mechanism and not the behavior.

**Prior art.** The live slice tests build a real repository with an
origin and drive the loop end to end; the retry-then-pass and
retry-then-fail cases are the closest existing analogue to this
feature's two headline behaviors. The git package's tests build a
temporary repository per case with a helper, which is the pattern the
`sliceguard` tests follow. The slices package's parser tests show the
table-driven style used for the exemption cases. All tests use the
standard library only, which matches the repo's existing convention.

## Out of Scope

- **Semantically dead code.** Code that is still called but provably
  inert stays with the cli dead-code audit. That audit reasons about
  meaning, which is judgment, and it remains the net for cases a
  deterministic guard cannot see.
- **Changes to the cli dead-code audit.** The runtime now catches the
  slice case upstream, so the audit needs no change. It keeps its
  current behavior, including its comment on a red verdict.
- **Reporting on a successful repair.** A reuse repair loses nothing, so
  the run stays silent. No pull request line, no issue comment.
- **Non-Go repositories.** The guard reports nothing for a repo it
  cannot analyze. Language support beyond Go is a separate decision.
- **A slice that edits an earlier slice's lines.** Only abandonment and
  file deletion are covered. A slice that legitimately edits earlier
  code inside its own scope must not be flagged.
- **The research self-consistency defect.** The PRD for issue #31 named
  a logging pattern as the cause of silent failure, then prescribed that
  same pattern for the two automatic paths. Nothing in the chain checks
  a PRD against itself. This is a research defect, not a slicing one,
  and it needs its own PRD.
- **Pull request #727 and issue #31.** This PRD does not merge, re-run
  or amend them.
- **Enabling slice builds in other repos.** Only the cli program sets
  the slices flag today. Rollout is a separate decision.

## Further Notes

**Deploy chain.** A runtime change is not live until it is tagged and
each cli workflow pins the new version. The install line is copy-pasted
across seven workflow files in the cli repo, with no shared composite
action, so a release is a seven-file bump. This is the same chain that
`memory/minions_deploy_chain.md` records.

**Unreleased work already on the branch.** The partio-minions working
copy sits one commit past `v0.0.13`, on the `argos/resume-marker-range`
branch. That commit fixes slice-marker counting so markers are counted
on the branch and not on its base. It is not in any released tag, so the
tag that carries this feature carries it too. The release notes must say
so.

**No release tooling.** The Makefile builds, tests and lints, but it
never creates a tag. The de-facto process is to publish a GitHub Release
at the merge commit, then bump the cli pin in a follow-up pull request.
A known failure mode is recorded in the earlier per-slice rollout notes:
`go install` can hit a checksum-database error on a minutes-old tag, so
the first run after a release can fail for a reason unrelated to the
change.

**Why the guard is deterministic.** The house rule is code over prompts
when the output is deterministic. Whether a declaration has a reference
is a fact about the tree, so it is a program. The cli dead-code audit
uses a Claude session because it hunts inert-but-called code, which is a
judgment. Both are correct for what they judge.

**Why this is not solved by a better instruction.** The `implement`
program already tells each slice to build on earlier slices and never
redo or undo them. Slice 2 of issue #31 read that instruction and redid
the work anyway. The prompt change in this PRD gives the slice facts it
did not have. The guard is what holds when the facts are not enough.
