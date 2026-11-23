# 62: Windows recording: audio, webcam, pause, cursor and click events

## Goal

A creator on Windows records a complete tutorial: screen, microphone, computer sound and webcam, with pauses, each on its own track, and with cursor and click data saved so later finishing needs no second take.

## Context and related decisions

- Author decision (2026-11-23): the recording MVP is screen, microphone, system audio, webcam on its own track, pause, cursor and click events saved, and direct placement on the timeline.
- ADR-0049, SDD-023 (epic 61), `docs/planning/screen-recording/research.md` section 3.

## User stories

- As a creator, I want my voice and the computer sound on separate tracks so that I can balance them later.
- As a creator, I want my webcam on its own track so that I can move or remove it in the editor.
- As a creator, I want to pause and resume so that I do not need to cut dead time.
- As an agent, I want the click timeline of a recording so that I can propose zooms.

## Scope

- Modules: `capture/`, `core/recording/`, `engine/capturehost/`, `service/` recording, the adapters, the record dialog, SDD-023.
- Must not touch: the timeline schema, `renderer/`.

## Out of scope

Applying zoom or cursor effects (65), layouts (66), per-application audio capture.

## Dependencies

61.

## Surfaces

| Surface | Change |
| --- | --- |
| Desktop, Windows | Full |
| Desktop, macOS and Linux | Unavailable until 67 and 68 |
| Web | Not affected |
| HTTP, CLI | Every operation below |
| MCP | Read through existing tools; no new tool |

## Core commands and permissions

Names are proposals until task 1 writes them to SDD-000.

| Operation | Scope | Who |
| --- | --- | --- |
| `RecordingSources` (extended with microphones and cameras) | `read` | Anyone with the scope |
| `RecordingPause`, `RecordingResume` | `media` | **Pause:** anyone with the scope. **Resume:** a person only, since it restarts capture |
| `RecordingEvents` | `read` | Anyone with the scope |
| `RecordingPlace` (extended to several tracks) | `edit` | One batch |

## Technical design

- **Files:** one file per source (screen, microphone, system audio, webcam), all stamped from one clock (QPC on Windows). Placement aligns them on the timeline.
- **Audio:** WASAPI capture for the microphone and WASAPI loopback, in shared mode, for the system mix. Protected content may be silent, and the UI says so.
- **Webcam:** Media Foundation capture on its own file.
- **Pause:** sources stop writing; the clock offset is recorded so tracks stay aligned.
- **Events:** a JSONL side file of pointer position, button down and up, with timestamps on the shared clock, read through raw input. No keystrokes are recorded. Coordinates are relative to the captured source.
- Events stay local and are deleted with the recording.

## Tasks

- [ ] 1. SDD-023 and SDD-000: tracks, the shared clock, the event format and its privacy rule. Verify: `task lint:md` exits 0.
- [ ] 2. Core rules: tracks, pause segments, event validation and limits. Verify: `go test ./core/recording/...` passes.
- [ ] 3. Microphone and system audio in the helper. Verify: the Rust gate in `capture/`, and an integration test that records a known tone and finds it in the output.
- [ ] 4. Webcam on its own file. Verify: an integration test with a virtual or real camera, skipped with a stated reason when no camera exists.
- [ ] 5. Pause and resume across all sources. Verify: tests with a fake helper, and a real capture whose tracks stay aligned after three pauses.
- [ ] 6. Cursor and click events by raw input. Verify: an integration test with synthetic input, and a test that no key event appears in the file.
- [ ] 7. Service, adapters and parity. Verify: `go test ./service/... ./web/...`, `go test -run TestSecurity_ ./...` and `go run ./internal/tools/surfacebudget` pass.
- [ ] 8. The dialog: device selection, level meters, pause. Verify: `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n` and `task test:web` pass.

## Test strategy

Known-tone and flash fixtures to measure sync; fake helper for service tests; real device tests on Windows, skipped with a reason when the device is missing.

## Acceptance criteria

1. **Given** a recording with all four sources, **when** it is placed, **then** the timeline holds one video track for the screen, one for the webcam and one audio track per sound source.
2. **Given** a flash and a tone produced at the same instant, **when** a 10-minute recording with three pauses is measured, **then** drift between tracks is at most 40 ms (NFR-068 fixture).
3. **Given** a recording with clicks, **when** `RecordingEvents` is read, **then** each synthetic click appears within 20 ms of its time, at the right coordinates.
4. **Given** typing during a recording, **when** the event file is inspected, **then** it holds no key data (security test).
5. **Given** an agent session, **when** it calls `RecordingResume`, **then** the answer is `E_PRIVILEGED_SESSION`.
6. `task lint`, `task test`, `task test:web` and the Rust gate pass.

## Risks and mitigation

- Raw input from a lower-integrity process may miss events over elevated windows: documented as a limit [to validate].
- Drift between devices: measured with the fixture, corrected on placement from the shared clock.
- **STOP** if any approach requires a global keyboard hook, if drift cannot meet 40 ms, or if a new crate is outside ADR-0049's list.

## Size

L: four sources and one clock, with real-device tests.

## Priority

Must. It completes the MVP the author chose.

## Hand-off for review

The commits, the sync measurements, each criterion with its command and result, and the devices actually tested.
