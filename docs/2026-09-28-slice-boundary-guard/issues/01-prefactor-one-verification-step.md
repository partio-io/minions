# 01 — Prefactor: route both check runs through one verification step

**Source PRD**: [../prd.md](../prd.md)
**Blocked by**: None — can start immediately

## What to build

A reshape of the executor's check-and-retry path. Behavior does not
change. The full test suite is green before the first edit and green
after the last.

Today the retry helper takes eight positional parameters and runs the
deterministic checks twice — once before the fix session and once
after. Two of those parameters exist only to label output, and the
helper branches on whether a slice number is set.

The later slices add a second kind of verification that needs the slice
plan, the slice number and a base ref. Threaded positionally, that is
three more parameters on two functions, and the non-slice caller passes
zero values for all of them.

Reshape the path so both check runs go through one composable
verification value. The retry helper asks that value to verify, uses
the combined pass flag and failure text exactly as it does now, and
asks it to verify a second time after the fix session. Adding another
verification later becomes a composition, not a signature change.

Both existing call sites keep working: the plain agent path and the
slice loop. Their observable behavior is identical.

## User stories covered

PRD stories 20, 27.

## Acceptance criteria

- [x] The full test suite passes before any edit in this slice.
- [x] The retry helper runs verification through a single value rather
      than two direct calls to the deterministic check runner.
- [x] The verification value carries the labeling context that the
      `sliceNum > 0` branch handles today, so the helper no longer
      branches on a sentinel number to build its scope string.
- [x] The plain agent path and the slice loop both call the reshaped
      helper and produce the same output as before.
- [x] A failing check still produces exactly one fix session, then one
      re-verification, and returns the second result.
- [x] The retry prompt text, the retry turn budget and the debug log
      file naming are unchanged.
- [x] No new exported symbol leaves the executor package.
- [x] No behavior change is introduced: no new failure route, no new
      configuration key, no new output line on a passing run.
- [x] The full test suite passes after the last edit in this slice.

## Modules touched

- `internal/executor` — the check-and-retry path and its two call sites.

## Test prior art

The executor package's existing tests cover the retry path end to end
through the live slice-loop test file: look for the retry-then-pass and
retry-then-fail cases, which build a real repository and drive the loop.
This slice adds no new test; it must leave those cases green unchanged.
The plain agent path is covered in the package's main executor test
file.

## Out of scope

- Any use of the new verification value beyond the two existing checks.
  Issue 03 adds the guard to it.
- Any change to the deterministic check runner itself or to the
  per-repo check configuration.
