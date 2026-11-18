# Plan 025: Command core fuzz, crash tests and the one-hour soak of the service

This plan answers whether the command core and the service survive long campaigns: `FuzzApplyBatch` for 30 minutes on each operating system, the crash suite 100 times with random crash points and a 10 minute kill test, and a one-hour soak with four keys. When delivered, no `E_INTERNAL` is returned, every confirmed revision is in the log, the database integrity check is `ok`, and the commit, round-trip, event and CLI-read budgets hold.

- Owner: senior-a (core fuzz and the SDD-002 freeze); senior-c (service campaigns)
- Effort: M
- Depends on: Linux and macOS runners and an idle reference machine; the author for freezing SDD-002 for 1.0
- Origin: EPIC-28 (P4-ENG-001): TASK-0133; EPIC-31 (P4-SRV-002): TASK-0144, TASK-0146, TASK-0148
- Status: TODO
- Written against: 8787714a

## 1. Why

The core and the service are the base of the product; a corrupted document or a leaked job at 1.0 would lose user work. Only Windows and Linux ran the core fuzz; the crash suite ran 100 times only on Windows (355 s); the one-hour soak never ran; the commit budget was measured under load (26.2 ms against 25 ms).

## 2. Current state (drift check)

```text
go test ./core/... -race -cover
go test ./core/command/ -fuzz FuzzApplyBatch -fuzztime 30s
go test ./service/...
ls service
git log --oneline -n 10 -- core/command service
```

Already true (from the former card statuses):

- `FuzzApplyBatch` ran 30 minutes on Windows (149k executions) and Linux (168k), no findings; an `item.move` slide bug was fixed (82ec7429..94f90492, release/v0.22.0). The `Determinism` job runs on three operating systems and the hashes of Windows equal Linux (1,297 cases) (ba3ba9e1 + 4964c052).
- Core review (ce22a2a0): `go test ./core/... -race -cover` ok (command 96.1%, lint 96.4%, plan 97.0%), lint 0 findings, no `time.Now` or `rand` in the core.
- Open: freezing SDD-002 for 1.0 (the author decides; its status says "frozen for the ALPHA"), the fuzz on macOS, and benchmarks on an idle machine (`ApplyAndLintFast` 17.6 ms busy against 15 ms).
- The service: 100 crash runs on Windows only (355 s); the 10-minute kill test and the three-OS run are not recorded; commit p95 26.2 ms measured under load against 25 ms; the SDD-005 numbers were measured under load; the one-hour soak never ran (one local 10-minute run exists).
- The path `service/crashtest/` named by the former cards did not exist when this plan was written (see STOP conditions).

## 3. Steps

### Step 1: Fuzz the core on the third system and freeze SDD-002

Owner: senior-a. Files: `core/command/`, `core/plan/`, `core/lint/`, `docs/sdds/002-command-catalogue.md`.

Acceptance:

- [ ] `go test ./core/command/ -fuzz FuzzApplyBatch -fuzztime 30m` finds no failure on each runner (Windows, Linux, macOS); `E_INTERNAL` is never returned and `timeline.Validate` holds after every step.
- [ ] `go test ./core/command/... ./core/plan/... ./core/lint/...` passes.
- [ ] Docs updated: `docs/sdds/002-command-catalogue.md` (status: frozen for 1.0, after the author agrees) and `CHANGELOG.md`.

Commit: `test(core): <english summary>` per new test, or none if only runs.

### Step 2: Add the BETA import and export points to the crash suite

Owner: senior-c. Files: crash test files of `service/` (see STOP conditions), `service/soak_test.go`.

Include the bundle import and export crash points added in the BETA, and a 10 minute kill test (`SIGKILL`, `taskkill /F`).

Commit: `test(service): <english summary>`.

Acceptance:

- [ ] `go test ./service/crashtest/...` passes.
- [ ] The crash suite runs 100 times on each operating system with randomised crash points.

### Step 3: Run the one-hour soak

Owner: senior-c. Files: `service/soak_test.go`.

An in-process and HTTP soak of one hour with four keys running mixed operations and exports.

Commit: `test(service): <english summary>`.

Acceptance:

- [ ] Goroutine count and resident memory return to within 10% of the baseline (starting value, confirmed by this measurement); no `5xx` other than the documented `503` for a locked workspace; no job left `running` after shutdown.

### Step 4: Prove the final service budgets

Owner: senior-c. Files: `service/commit_bench_test.go`, `docs/sdds/005-data-model-and-persistence.md`, `CHANGELOG.md`.

Acceptance:

- [ ] `go test -tags crashtest ./service/crashtest/... -count=100` passes on the three runners: every confirmed revision is in the log and `PRAGMA integrity_check` is `ok` (NFR-013).
- [ ] Commit p95 is at most 25 ms and round trip p95 at most 50 ms on the 200-item project with a 100,000-row log (NFR-001; SDD-005 section 18 check 12); event p95 at most 20 ms (NFR-002); `BenchmarkCLIRead` p95 at most 300 ms (NFR-024), on an idle machine.
- [ ] The soak ends with goroutines and memory inside the declared margin and zero `running` jobs (`TestSoak`, nightly job owned by the release track).
- [ ] Docs updated: `docs/sdds/005-data-model-and-persistence.md` sections 4 and 14 (final numbers) and `CHANGELOG.md`; `task lint` reports no findings.

## 4. Out of scope

- New commands in the core (plan 033).
- The nightly workflow itself (plan 030).
- Security fuzz targets (plan 022).

## 5. STOP conditions

- `service/crashtest/` does not exist in the tree: stop and ask the orchestrator where the crash tests live or whether to create the package (the cards name it; the tree did not have it).
- The commit p95 is above 25 ms on an idle machine: fix the cause with a test or stop and report; do not raise the limit.
- A fuzz run finds a crash: the fix and its regression test come first; restart the 30 minutes.
- Freezing SDD-002 without the author's yes: stop.

## 6. Maintenance

Re-run the campaigns at every release candidate and after any new command or persistence change. New crash points are added in the same pull request as the operation that introduces them.
