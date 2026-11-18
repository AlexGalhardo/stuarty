# Plan 021: Golden frames and export performance on three operating systems

This plan answers whether the composited frames equal the stored golden hashes on Windows, Linux and macOS, and whether the export and ingest budgets hold on the three CI runners and on the idle reference machine. When delivered, SDD-004 sections 12 and 13 hold the numbers of all three runners with their commits, and every metric is above its limit or has a fix with a test.

- Owner: senior-b
- Effort: L
- Depends on: Linux and macOS runners and an idle reference machine with every agent stopped; plan 027 for the runs with the own FFmpeg build
- Origin: EPIC-01 (P1-RND-004): TASK-0007; EPIC-05 (P1-QA-002): TASK-0026; EPIC-29 (P4-RND-001): TASK-0134, TASK-0135, TASK-0137, TASK-0138
- Status: BLOCKED (measures a performance budget; needs the reference machine idle with no agents running, and Linux/macOS runners)
- Written against: 8787714a

## 1. Why

The three reference scenes were proven equal to the golden hashes on Windows only, and only the Windows performance table is in SDD-004 section 12 (2.14 to 2.72 times real time, but measured under load). Release 1.0 needs the numbers of the three runners (NFR-010) and the end-to-end wall time of success metric 4 of PRD-001.

## 2. Current state (drift check)

```text
go test ./engine/export/... ./engine/compositor/... ./engine/audio/...
task lint
git log --oneline -n 10 -- engine/export internal/tools/perfreport
```

Already true (from the former card statuses):

- Go lint and the golden comparison are proven on Windows (senior-b, 2026-11-16).
- Export, compositor, mixer and verification are integrated (PR #62); the Go process stays below 500 MB during a 60 s export (NFR-007, measured by a test); a truncated output fails verification and leaves no file in `exports/`.
- `internal/tools/perfreport` and the benchmarks exist; the Windows table is in SDD-004 section 12, declared as limited by load.
- A 60 s export took 55.7 s with libopenh264 (not a hardware encoder) on a loaded machine; that time is not valid, and 1.5 times real time was not met under load.
- The only gate on `BenchmarkExportCommonCase` is the floor of 1.5; there are no limits derived from measured numbers.
- Not run since CI was paused: golden comparison on Linux and macOS; `engine/export/bench_release_test.go` and `engine/ingest/bench_release_test.go` are listed as the files for the release benchmarks.

## 3. Steps

### Step 1: Golden comparison on the three operating systems

Owner: senior-b. Files: `engine/export/testdata/`, `engine/export/`.

Run the export tests on a Linux and a macOS runner (perfreport on an idle machine for the timings).

Commit: `test(engine): <english summary>` or none if only measurements.

Acceptance:

- [ ] `go test ./engine/export/... ./engine/compositor/... ./engine/audio/...` passes; the composited frames of the three reference scenes equal the golden hashes on all three operating systems (NFR-010).
- [ ] `go test ./engine/export/...` compares 30 frames per scene against the golden set on all three operating systems.

### Step 2: Run the release benchmarks on the three runners and the reference machine

Owner: senior-b. Files: `engine/export/bench_release_test.go`, `engine/ingest/bench_release_test.go`, `docs/sdds/004-render-pipeline.md`.

Run the P2-RND-014 benchmarks on the release candidate on Linux, macOS and Windows runners and on the reference machine, plus the BETA scenes (blur, 60 fps, 4K where supported, GIF, animated image), and record each number in SDD-004 section 12 with its commit.

Commit: `feat(engine): <english summary>` for any new benchmark; the numbers go in `docs/sdds/004-render-pipeline.md` (`docs(sdd): record the release benchmark numbers`).

Acceptance:

- [ ] `go test ./engine/...` passes.
- [ ] `BenchmarkExportCommonCase` is at least 1.5 times real time on the reference runner, and the numbers of the three runners are recorded in SDD-004 section 12.
- [ ] Every number in section 12 carries the commit it was measured at; SDD-004 sections 12 and 13 are updated with the numbers of the three runners.

### Step 3: Measure the end-to-end wall time of success metric 4

Owner: senior-b. Files: `engine/export/bench_release_test.go`, `engine/ingest/bench_release_test.go`.

Measure a 60 second vertical video with automatic captions, from import to exported file (PRD-001 section 6 metric 4), with the `small` transcription model on the reference machine.

Acceptance:

- [ ] The video goes from import to exported file in under 5 minutes of wall time on the reference machine with `small`.

### Step 4: Fix any metric below its limit

Owner: senior-b. Files: `engine/` (the responsible code).

For each metric below its limit, fix the responsible code with a test, or open a follow-up and block the release.

Commit: `fix(engine): <english summary>` per fix, each with its test.

Acceptance:

- [ ] Each fix has a test that fails before and passes after, or a follow-up is recorded and the release is marked blocked.

### Step 5: Prove the final budgets

Owner: senior-b. Files: `docs/sdds/004-render-pipeline.md`, `CHANGELOG.md`.

Acceptance:

- [ ] The export of the 60 s, 1080x1920, 30 fps project with captions finishes in at most 60 s with a hardware or operating system encoder, and the Go process peaks below 500 MB (NFR-006, NFR-007) on the reference machine.
- [ ] `BenchmarkExportCommonCase` is at least 1.5 times real time on all three runners; cold `ViewFrame` is within 500 ms at UI width and 2 s at agent width (NFR-004, NFR-023); ingesting a 60 s 1080p clip takes at most 30 s (NFR-005).
- [ ] The docs are updated: `docs/sdds/004-render-pipeline.md` section 12 (final numbers) and `CHANGELOG.md`; `task lint` reports no findings.

## 4. Out of scope

- The Rust renderer and its parity tests (renderer epics).
- Browser-side latency (plan 020).
- The own FFmpeg build itself (plans 026 and 027).

## 5. STOP conditions

- The machine is not idle or an agent is running: stop; the numbers are not valid.
- No hardware or operating system encoder is available on the reference machine: stop and ask the orchestrator before using libopenh264 for the 60 s limit.
- The Rust renderer replaces the Go compositor before this plan runs: the golden set and the benchmarks change; ask the orchestrator which one is measured.

## 6. Maintenance

Re-run the benchmarks at every release candidate and after any change to the compositor, mixer, ingest or encoder flags. A golden hash changes only through the golden regeneration procedure, never by editing the expected value.
