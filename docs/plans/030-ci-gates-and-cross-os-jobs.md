# Plan 030: Prove the CI gates, the nightly workflow and the cross-OS jobs once CI runs again

This plan answers whether the CI pipeline enforces what SDD-009 promises: the Go suite passes with `-race` on three operating systems, the generated-files and coverage gates fail when they should, the median workflow time stays under 15 minutes, the nightly workflow reports the soak and the benchmarks, and the Firefox, WebKit and cross-OS bundle jobs pass and are required checks. When delivered, each gate has been seen failing on a test branch and passing on `main`.

- Owner: senior-d; the author applies the ruleset change
- Effort: M
- Depends on: the author turning real CI back on (the pull-request workflows are disabled and the Go suite is paused on Ubuntu and Windows through `GO_SUITE`); the author for the ruleset required checks
- Origin: EPIC-14 (P2-REL-005): TASK-0067; EPIC-27 (P3-REL-005): TASK-0127
- Status: BLOCKED (needs a real CI run, a paid API key, a clean machine or another system, or the author)
- Written against: 8787714a

## 1. Why

The gates exist but were never demonstrated: `Generated files` was never shown to fail on a manual edit; the coverage gate step was skipped since the pause and no test branch was shown; `scripts/ci-time.sh` was not run; `nightly.yml` never ran; the `bwrap` job and the benchmark-failure case were never exercised; the Firefox, WebKit and `bundle-matrix` jobs never ran on a pull request.

## 2. Current state (drift check)

```text
bash scripts/ci-time.sh --help
gh workflow list
gh run list --workflow ci.yml --limit 10
git log --oneline -n 10 -- .github/workflows/ci.yml
```

Already true (from the former card statuses):

- CI gates, the nightly workflow and `ci-time.sh` are integrated; the workflow steps of the Firefox, WebKit and bundle jobs are green; 3 lines of the gate card were met.
- `covergate` has a unit test for a core package below 90 percent (`TestRun` in `internal/tools/covergate`).
- `TestSecurity_T16_BwrapHasNoNetwork` exists; the `bwrap` job checks `PASS` and not `SKIP`.
- `release-verify.sh` and `desktop-verify.sh` were not run here (no cosign).
- The six cross imports need the three CI systems; only a local Windows run was done.

## 3. Steps

### Step 1: The Test job on three systems with `-race`

Owner: senior-d. Files: `.github/workflows/ci.yml`, `Taskfile.yml`.

Re-enable the Go suite on Ubuntu and Windows (`GO_SUITE`) and run it.

Commit: `ci(cli): re-enable the Go suite on all three runners`.

Acceptance:

- [ ] The `Test` job passes on `ubuntu-latest`, `windows-latest` and `macos-latest` with `-race` (NFR-045; SDD-009 section 20 check 7).

### Step 2: Show the generated-files and coverage gates failing and passing

Owner: senior-d. Files: `.github/workflows/ci.yml`, `Taskfile.yml`, `internal/tools/covergate`.

Use test branches that are not merged.

Acceptance:

- [ ] `Generated files` fails when a generated directory is edited by hand and passes after `task generate` (check 8).
- [ ] The coverage gate fails when a `core/` package falls below 90% on a test branch and passes on the tip of `main`.

### Step 3: Nightly workflow, `bwrap` and benchmark gates

Owner: senior-d. Files: `.github/workflows/nightly.yml`, `.github/workflows/ci.yml`.

Acceptance:

- [ ] `gh workflow run nightly.yml` finishes and its summary lists the soak test and the benchmarks with their numbers.
- [ ] The `bwrap` job runs `TestSecurity_T16_BwrapHasNoNetwork` not skipped (the test output is checked for `PASS`, not `SKIP`); the benchmark job fails on a test branch that adds a sleep to the compositor.

### Step 4: Firefox, WebKit and the bundle matrix

Owner: senior-d. Files: `.github/workflows/ci.yml`.

Acceptance:

- [ ] The two browser jobs pass on a pull request and appear in the required checks of the ruleset (the author applies the ruleset change).
- [ ] `bundle-matrix` passes: six imports (each system imports the bundles of the other two) with equal document and media hashes.

### Step 5: The CI time budget

Owner: senior-d. Files: `scripts/ci-time.sh`.

Acceptance:

- [ ] Over 10 consecutive pull requests, the median total time of the `CI` workflow is under 15 minutes and no run exceeds 20 minutes (`scripts/ci-time.sh`; NFR-042; SDD-009 section 20 check 6), and `scripts/ci-time.sh` keeps reporting a median under 15 minutes in 10 runs.
- [ ] `task lint` reports no findings.

Commit: `test(cli): <english summary>` or none if only measurements.

## 4. Out of scope

- The release workflow and the 1.0 audit (plan 031).
- Making a slow test faster beyond what the budget needs.
- Editing the ruleset (the author does it).

## 5. STOP conditions

- The author has not turned CI back on: stop.
- The median exceeds 15 minutes: report the slowest jobs; the last audit of release v0.20.0 found a median of 15.5 min and a maximum of 34.8 min; do not raise the limit.
- A gate cannot be shown failing without merging the bad change: stop; never merge a deliberately broken branch.

## 6. Maintenance

Any new workflow job states its place in the time budget. Re-run `scripts/ci-time.sh` at every release candidate. A new required check is added to the ruleset by the author in the same week as the workflow.
