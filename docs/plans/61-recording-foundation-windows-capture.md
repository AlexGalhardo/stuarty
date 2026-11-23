# 61: Recording foundation and Windows screen capture

## Goal

A creator on Windows records the screen from Stuarty and finds the recording on the timeline of the project, with no second tool. This epic lays the domain, the helper and the contract every later recording epic builds on.

## Context and related decisions

- Author decisions (2026-11-23): desktop gets native capture and the web gets basic recording; the order of systems is Windows, macOS, Linux; capture runs in a separate native helper; an agent never starts a capture alone.
- ADR-0049 (native capture helper) and ADR-0051 (agent consent for capture), both accepted by author decision D-133. A recording enters the project only after a person confirms it, on a step that says agents with project access can read it.
- PRD-001 section 7 excludes "Recording camera, screen or voice": task 2 amends it.
- ADR-0011 and SDD-007 section 6 (the FFmpeg process policy), SDD-007 section 6.10 (the native GPU host, the precedent for a native child process), ADR-0032 (no outbound).
- `docs/planning/screen-recording/research.md` section 3.

## User stories

- As a creator, I want to record my screen from the project so that I do not need a second tool.
- As a creator, I want a recording to survive a crash so that I do not lose a take.
- As an agent, I want to read the state of a recording so that I can continue the work when it ends.

## Scope

- Modules: `spikes/capture-windows/`, `capture/` (new, the helper), `core/recording/` (new), `engine/capturehost/` (new), `service/`, the store and a migration, the HTTP, MCP and CLI adapters, a minimal record dialog in `web/resources/js/`, `desktop/` packaging of the helper, PRD-001, PRD-004, SDD-000, SDD-007, SDD-023 (new).
- Video only, one display or one window, on Windows.
- Must not touch: `renderer/`, the export pipeline, the vault.

## Out of scope

Audio, webcam, pause and cursor events (epic 62), web capture (63), agent preparation (64), macOS (67), Linux (68).

## Dependencies

57 (a password session is what makes a person in local mode) and the choice of the redesign direction. **Task 1, the spike, has no UI and no product code, and can run before both.**

## Surfaces

| Surface | Change |
| --- | --- |
| Desktop, Windows | Full capture |
| Desktop, macOS and Linux | `E_CAPTURE_UNAVAILABLE` until 67 and 68 |
| Web and Docker | `E_CAPTURE_UNAVAILABLE` until 63 |
| HTTP, CLI | Every operation below |
| MCP | Through the existing `job_get`, `job_cancel` and `edit` tools. No new tool: `tools/list` is at the 23,400-byte limit of NFR-020 |

## Core commands and permissions

Names are proposals until task 2 writes them to SDD-000.

| Operation | Scope | Who |
| --- | --- | --- |
| `RecordingSources` | `read` | Anyone with the scope |
| `RecordingGet`, `RecordingList` | `read` | Anyone with the scope |
| `RecordingStart` | `media` | **A person in a privileged browser session only** (`E_PRIVILEGED_SESSION` otherwise) |
| `RecordingStop`, `RecordingCancel` | `media` | Anyone with the scope: stopping a capture is always safe |
| `RecordingPlace` | `edit` | Places a finished recording on a timeline as one batch |

## Technical design

- **Helper:** `stuarty-capture`, a Rust executable in `capture/`, started by `engine/capturehost` under the process policy of SDD-007 section 6: absolute path, minimal environment, run directory, deadline, kill of the process tree.
- **Protocol:** versioned JSON control messages on stdin and stdout; frames go to an FFmpeg child through a pipe. The helper decides nothing about the project.
- **Capture:** Windows Graphics Capture, minimum Windows 10 2004 for cursor control. DXGI is the fallback only if the spike requires it.
- **File:** fragmented MP4 written to the workspace, so an interrupted recording remains decodable. Encoder selection reuses `engine/ffmpeg/capability.go`.
- **State machine in `core/recording`:** `prepared`, `recording`, `finishing`, `ready`, `failed`, `cancelled`.
- **Result:** a ready recording becomes a library asset through the existing import path (`MediaImport`) and is placed with existing timeline commands.
- **Limits:** a maximum duration, a minimum free disk space checked at start and during capture.

## Tasks

- [ ] 1. Spike in `spikes/capture-windows/`: capture 1920x1080 at 60 fps for 10 minutes into fragmented MP4 through the pinned LGPL FFmpeg, and kill it once mid-way. Record dropped frames, CPU and the recovered duration on the reference machine. Verify: `ffprobe` opens both files, and the numbers are in the spike's report.
- [ ] 2. Amend PRD-001 section 7, add RF-430 to RF-442 and NFR-067, NFR-068 to PRD-004, write SDD-023 and the names in SDD-000, add the threats to SDD-007. Verify: `task lint:md` and `go run ./internal/tools/threatcheck` exit 0.
- [ ] 3. `core/recording`: the state machine and its limits, pure, with tests of every transition. Verify: `go test ./core/recording/...` passes.
- [ ] 4. The helper protocol and `capture/` with a fake source. Verify: `cargo fmt --check && cargo clippy --all-targets -- -D warnings && cargo test && cargo deny check` in `capture/`.
- [ ] 5. `engine/capturehost` under the process policy, with a fake helper: start, stop, kill, timeout, malformed message. Verify: `go test ./engine/capturehost/...` passes.
- [ ] 6. Windows capture in the helper. Verify: the helper's integration test writes a playable file that `ffprobe` reads on Windows.
- [ ] 7. The service operations, the store, the migration and the recovery of an interrupted recording at start-up. Verify: `go test ./service/...` passes.
- [ ] 8. HTTP, CLI and MCP with parity tests, and the security tests of the person rule. Verify: `go test ./web/...`, `go test -run TestSecurity_ ./...` and `go run ./internal/tools/surfacebudget` pass.
- [ ] 9. The record dialog (source picker, visible recording state, stop), packaging of the helper. Verify: `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n` and `task test:web` pass.
- [ ] 10. End to end on Windows: record, stop, place on the timeline. Verify: a scripted run with the real helper ends with a clip on the timeline and a matching duration.

## Test strategy

Unit tests of the state machine; a fake helper for the host and the service; one real capture test tagged for Windows; security tests for the person rule; a crash test that kills the helper and checks recovery.

## Acceptance criteria

1. **Given** a person session on Windows, **when** they record for 60 seconds and stop, **then** `ffprobe` reports a duration of 60 s plus or minus 0.5 s and the recording is a library asset.
2. **Given** a ready recording, **when** `RecordingPlace` runs, **then** one batch adds one clip and undo removes it.
3. **Given** the local token, an API key or an MCP session, **when** `RecordingStart` is called, **then** the answer is `E_PRIVILEGED_SESSION` and no helper process starts (`go test -run TestSecurity_ ./...`).
4. **Given** a recording in progress, **when** the helper is killed, **then** after restart the recording is `ready` with the frames written before the kill, or `failed` with a reason, and never stays `recording`.
5. **Given** less free disk than the limit, **when** capture runs, **then** it stops cleanly and keeps what was written.
6. **Given** 1920x1080 at 60 fps on the reference machine, **when** recording for 10 minutes, **then** at most 1% of frames drop (NFR-067).
7. With no connection configured, the no-egress test still passes.
8. `task lint`, `task test`, `task test:web` and the Rust gate of `capture/` pass.

## Risks and mitigation

- Antivirus flags an unsigned helper that captures the screen: measured in the spike.
- A/V and encoder load: encoder choice reuses the existing capability check.
- **STOP** if the spike misses NFR-067 on the reference machine, if capture needs cgo in the server, if a dependency outside ADR-0049's approved list is needed, or if `tools/list` would exceed its budget.

## Size

XL: a new domain, a new native executable and a new process host. If task 6 takes more than one sitting, the orchestrator splits tasks 9 and 10 into a follow-up epic.

## Priority

Must. Every other recording epic depends on it.

## Hand-off for review

The commits, the spike's numbers, each criterion with its command and result, how to run a recording on Windows, and anything not run.
