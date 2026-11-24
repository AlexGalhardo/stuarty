# 90: Real crash recovery: kill a real process in a publication, an upload and an export

## Goal

A creator whose computer dies, or whose process is killed, in the middle of publishing a post, uploading a video or exporting one, comes back to a workspace that is whole: nothing is posted twice, no half file is offered as a finished export, and the interrupted work resumes or ends with a clear reason. Today this is proved with a process that exits by itself or with a context that is cancelled; this epic proves it with a real process killed from outside.

## Context and related decisions

- Author decisions D-127 and D-135 (item 4): move the publishing environment into a support package, and kill a real process in the middle of a publication, an upload and an export, then prove recovery. This is the part of epic 42 that was left over; epic 42 keeps the measurements, the soak and the security review.
- What exists and what it proves, so that this epic does not repeat it:
  - `service/crashpoint` marks named places (`crashpoint.Hit`); with the `crashtest` build tag the point named by `STUARTY_CRASH_AT` ends the process with `os.Exit(137)`. The commit path, the bundle export and import, the migration, the emptying of the trash and the vault are covered (`web/app/servicetest/crash_test.go`, `crash_export_test.go`, `crash_import_test.go`, `crash_migrate_test.go`, `internal/tools/crashharness`, `bash scripts/crash.sh`, SDD-008 section 13.3). That process ends itself at a point the code chose.
  - `TestKillChild` and `TestKillRealServeDuringConcurrentHTTPCommits` (`web/app/servicetest/kill_test.go`, `kill_http_test.go`) kill a real process from outside (`Process.Kill`: `TerminateProcess` on Windows, `SIGKILL` elsewhere) in the middle of commits. Nothing kills a real process in a publication, an upload or an export.
  - The publication restart is proved by `TestSchedulerRestartMidPublishResumes` and `TestSchedulerStopsAPostThatKeepsInterrupting` (`service/publish/scheduler_test.go`) and by `TestSchedulerTwoServersOnTheSameWorkspacePublishOnce` (`web/app/servicetest/publication_test.go`), with a context that is cancelled and an in-memory store. A cancelled context runs the deferred cleanup of the process; a kill does not.
  - The publishing environment of the servicetests (`pubEnv`, `socialEnv`, `fakeNet`, `calendar`, `seedExport`, in `web/app/servicetest/publish_test.go`, `social_test.go`, `publication_test.go`) lives in `_test.go` files, so no other test package and no child program can import it. `service/testproject` is the precedent for a support package.
- SDD-020 section 7 (the YouTube upload session kept in `<workspace>/publish/uploads/<key>.json`) and section 27 (the lease, the interrupted attempt with the same upload key, `MaxAttempts` and the code `interrupted`), SDD-005 section 14.8 (the measured crash and kill suites) and section 18, SDD-008 section 13.3 (the pass rule), SDD-011 (the export job), SDD-007 section 6.4 (output is written under a generated name and moved into place with an atomic rename; `engine/export/job.go` publishes the `partial` file by rename).
- ADR-0032: the fake network is on loopback and the product opens no other connection. `.agents/rules/` and AGENTS.md section 3 rule 6 apply: no real network, no real account.

## User stories

- As a creator, I want a publish that was cut off to resume instead of posting a second copy, so that my channel does not show the video twice.
- As a creator, I want an export that was cut off never to appear as a finished file, so that I do not publish a broken video.
- As the team, I want the recovery proved with a real kill, so that "no committed change is lost" (NFR-013) holds for publishing and export too.

## Scope

- Modules: a new support package for the publishing environment (proposed name `service/testpublish`, next to `service/testproject`; it is for tests only), `web/app/servicetest` (the tests move to it with no change of behaviour, and the new kill tests are added), `internal/tools/crashharness` (the list of covered cases), `scripts/crash.sh`, SDD-008 section 13.3, SDD-005 section 14.8, SDD-020 section 27 (only if the proof shows the text is wrong).
- A fix that a kill test shows to be needed is a small commit of this epic when it is in one module and under about 150 changed lines; a larger one is a `VV` for the queue (`.agents/rules/plans.md` section 5) and the test stays, marked with the `VV` id.
- Must not touch: the renderer, the front end, the connectors' protocol code, the vault format.

## Out of scope

- A real network, a real social account or a real FFmpeg build other than the pinned one; power-loss simulation (the filesystem can roll back to an earlier commit, SDD-005 section 4, and is never inconsistent).
- The soak, the fuzz and the security review of epic 42.
- macOS and Linux hardware (epic 43); the tests must run on Windows and on Linux in Docker.

## Dependencies

42 (its crash harness is task 6, merged), 18, 20 (publication and the scheduler), 35 (export). Not in progress together with an epic that changes `web/app/servicetest/publish_test.go`, `social_test.go` or `publication_test.go`, or `service/publish/`.

## Surfaces

None: tests, a test-only support package, scripts and documents. No route, tool or screen changes.

## Core commands and permissions

None.

## Technical design

- **The support package.** `service/testpublish` holds what the publishing tests build: the fake network (an `httptest` server that records every request, can be told to stall or to cut a response, and keeps the bytes of an upload by session), the environment constructor (workspace, store, service with the fake connector registered, a project, a signed-in connection), the seed of a succeeded export, and a clock the tests move. The code moves as it is; the names stay; the existing servicetests import the package and pass unchanged. The package imports no `testing` helper that a child program cannot call, so it can also be used by a child process.
- **How a real kill is made deterministic.** The parent test starts the child (the test binary re-executed with a role variable, as `TestKillChild` does, or `stuarty serve` where the fake connector is not needed) with the workspace and the address of the fake network in its environment. The parent kills the child when it **observes** the moment, never after a sleep:
  - **Publication:** the fake network sees the first request of the send of a target (the lease is held, the attempt is 1, nothing is recorded as an end). The parent calls `Process.Kill`.
  - **Upload:** the fake network has received a chosen number of bytes of a resumable upload session (a fixed fraction of the file, with the session address stored in `publish/uploads/<key>.json`). The parent kills the child then.
  - **Export:** the child's export job has started FFmpeg and the temporary output exists and has grown (the parent polls the workspace file and the job row, with a deadline, never a fixed sleep). The parent kills the child; on Windows it also kills the FFmpeg process tree the child started so that no orphan keeps writing, and the test records whether an orphan existed.
- **What is asserted after each kill** (the pass rule of SDD-008 section 13.3, now with a real kill): the database opens and `PRAGMA integrity_check` is `ok`; a new process starts on the same workspace and on the same fake network; the lease of the interrupted target is taken again only after it ends; the target is sent once more with the same upload key and **the fake network counts one finished post** (a resumed session continues at the received offset, it does not start a second one); once `MaxAttempts` is used the target ends `failed` with the code `interrupted` and the network is not asked again; no export job stays `running` (it is resumed or `failed` with a stable code); no file at the final export name is shorter than its recorded size or has a different digest; a temporary file left behind is removed by the sweep; the cost reserved for a post that failed is released; the vault opens and holds every secret.
- **Where the kill tests live.** In `web/app/servicetest` with the `crashtest` tag, named `TestKillReal...`, run by `scripts/crash.sh` (the line `-run 'TestCrash|TestKill'` already matches them). `crashharness.TestEveryCrashPointHasARecoveryTest` also learns the kill cases, so a new publishing or export step without a kill case is flagged in review as the commit path is today.
- **Time.** The scheduler tests use the clock of the environment; the real-kill tests use the real clock with a short lease set through `SchedulerOptions.Lease`, so that waiting for a lease to end costs seconds, not minutes.

## Tasks

- [x] 1. Specify the cases and the pass rule: add the three real-kill cases, their observation points and their assertions to SDD-008 section 13.3 and the measured-suite note of SDD-005 section 14.8. Verify: `task lint:md` exits 0.
- [x] 2. Create `service/testpublish` by moving the publishing environment out of the `_test.go` files, with no change of behaviour. Verify: `go build ./service/testpublish/...` and `go test ./web/app/servicetest/... -run 'Scheduler|Publication|Publish|Social'` pass with the same list of tests as before (count them before and after; the numbers are in the commit message).
- [x] 3. Kill a real child in the middle of the send of a publication and prove recovery: lease, resume, one finished post, `interrupted` after `MaxAttempts`, the cost released. Verify: `go test -tags crashtest -count=1 -run 'TestKillRealPublication' ./web/app/servicetest` passes.
- [x] 4. Kill a real child in the middle of a resumable upload and prove that the session continues at the received offset. Verify: `go test -tags crashtest -count=1 -run 'TestKillRealUpload' ./web/app/servicetest` passes, and the fake network's log shows one session and a resumed offset.
- [x] 5. Kill a real child in the middle of a video export and of an image export; prove that no half file has the final name, that the job is not left `running`, and that the export can be run again to a complete file with the recorded digest. Verify: `go test -tags crashtest -count=1 -run 'TestKillRealExport' ./web/app/servicetest` passes (the video case runs with the pinned FFmpeg and skips with the reason printed when it is not installed; the image case never skips).
- [ ] 6. Repeat each kill case with the kill at several moments (three fractions of the upload, the start and the middle of the send, the start and the middle of the export) so that one lucky moment is not the proof. Verify: `go test -tags crashtest -count=3 -run 'TestKillReal' ./web/app/servicetest` passes.
- [ ] 7. Wire the cases into the harness and the script, and update the documents with what was measured (runs, durations, whether an orphan FFmpeg appeared on Windows). Verify: `bash scripts/crash.sh` exits 0 and `go test ./internal/tools/crashharness` passes.
- [ ] 8. Fix what the kills showed, or file it: each failing assertion is either fixed with the test that found it or recorded as a `VV` with the test marked. Verify: the commands of tasks 3 to 5 pass, or each remaining failure names its `VV`.

## Test strategy

The new tests are the strategy: real process, real SQLite store on a real workspace, fake network on loopback, kills at observed moments, repeated. The existing crash and servicetest suites must pass unchanged after task 2 (the move is the riskiest part, so it is its own commit). Run with `-race` is not used for the child; the parent assertions are plain.

## Acceptance criteria

1. `go test -tags crashtest -count=3 -run 'TestKillReal' ./web/app/servicetest` passes on Windows and, in Docker, on Linux (the evidence of each is in the hand-off).
2. After a real kill in a publication or an upload, the fake network counts exactly one finished post for the target, and the target is `published` or, after `MaxAttempts`, `failed` with `interrupted`, never sent again.
3. After a real kill in an export, no file with the final name differs in size or digest from the one the job records, and no job is `running` after the recovery of start-up.
4. `go test ./web/app/servicetest/... -run 'Scheduler|Publication|Publish|Social'` runs the same number of tests before and after the move, all passing, and no `_test.go` file defines `fakeNet` any more.
5. `bash scripts/crash.sh` exits 0; `go test ./internal/tools/crashharness` passes; SDD-008 section 13.3 describes the three new cases.
6. `task lint` and `task test` pass; no new dependency; no outbound connection other than loopback (`go test -run TestSecurity_ ./...` passes).

## Risks and mitigation

- A kill at a moment the test does not see: every wait has a deadline and the test fails with what it saw; there is no sleep as a synchronisation.
- On Windows an orphan FFmpeg may keep writing after the parent is killed. Mitigation: the test kills the tree and also asserts that, if an orphan is left alone, the final name is never written by it (the export writes to a temporary name and renames).
- The move of the environment breaks tests that other epics are editing. Mitigation: the dependency says not to run together; the move is one commit with the counted list of tests.
- **STOP** if a kill shows data loss or a double post that needs a change of the contract of SDD-020 or SDD-005 (not a fix inside one module): record the question for the author with the options.
- **STOP** if the proof needs a new dependency (a process-control library, a fault-injection tool): the standard library and `os/exec` are enough; anything else is the author's decision.
- **STOP** if the video export case cannot be made deterministic without the pinned FFmpeg present on the machine: keep the image case, record the video case as not run with the reason, and ask the orchestrator to run it where FFmpeg is installed.

## Size

M: a mechanical move, then three kill scenarios with their assertions, repeated.

## Priority

Should. Publishing and export are where a crash costs the creator most (a duplicate public post, a corrupt video), and today they are proved only with a simulated crash.

## Hand-off for review

The commits (the move is its own commit, with the before-and-after test counts); how to run the kill tests on Windows and in Docker; for each case the number of runs, the moments of the kill and the assertions that passed; whether an orphan FFmpeg appeared; the findings fixed and the `VV` entries filed; anything that contradicts SDD-005, SDD-008 or SDD-020.
