# 63: Basic web recording

## Goal

A creator using Stuarty in a browser, on Docker or a VPS, records the screen, camera and microphone without installing the desktop application.

## Context and related decisions

- Author decision (2026-11-23): the web gets basic recording, without zoom from clicks.
- ADR-0007 (thin client), SDD-012, ADR-0032. `docs/planning/screen-recording/research.md` section 3.

## User stories

- As a creator on a VPS, I want to record from my browser so that I can use Stuarty without the desktop app.
- As a creator, I want to know before I start what my browser cannot capture so that I do not lose a take.

## Scope

- Modules: `web/resources/js/` (a recorder module and the dialog), `service/` recording, a new upload route in the HTTP adapter, SDD-023, SDD-007.
- Must not touch: `capture/`, `engine/capturehost/`, the timeline schema.

## Out of scope

Global cursor and click data, system audio guarantees, Chrome on Android, finishing effects.

## Dependencies

61 (the domain and `RecordingPlace`). It can run side by side with 62 only if they do not change the same service files; otherwise after it.

## Surfaces

| Surface | Change |
| --- | --- |
| Web on HTTPS or localhost | Screen, camera, microphone; system audio where the browser offers it |
| Web on plain HTTP | The feature is disabled and the page explains why |
| Desktop | Keeps native capture |
| HTTP | Upload route |
| CLI, MCP | Read state only: a browser capture cannot be driven from outside |

## Core commands and permissions

Names are proposals until task 1 writes them to SDD-000.

| Operation | Scope | Who |
| --- | --- | --- |
| `RecordingUploadBegin`, `RecordingUploadChunk`, `RecordingUploadFinish` | `media` | A person in a privileged browser session |
| `RecordingGet`, `RecordingPlace` | as in 61 | as in 61 |

## Technical design

- `getDisplayMedia` and `getUserMedia` on a user gesture; `MediaRecorder` with `start(timeslice)`.
- Chunks go to the server as they are produced, in order, with an index and a checksum, so a closed tab loses at most the last chunk.
- The browser computes no edit: the server assembles the file with FFmpeg and creates the asset.
- The dialog reads what the browser supports through `MediaRecorder.isTypeSupported` and states it before recording.
- Upload limits reuse `STUARTY_MAX_IMPORT_BYTES`. The current product has no upload route of this kind [to validate in task 1].

## Tasks

- [ ] 1. SDD-023, SDD-000 and SDD-007: the upload contract, limits and threats. Verify: `task lint:md` and `go run ./internal/tools/threatcheck` exit 0.
- [ ] 2. Service and upload route: order, checksum, limits, resume after a dropped connection. Verify: `go test ./service/... ./web/...` and `go test -run TestSecurity_ ./...` pass.
- [ ] 3. The recorder module and its capability check. Verify: `bun run typecheck`, `bun run check:thin-client` and `task test:web` pass.
- [ ] 4. The dialog, the secure-context message and the visible state. Verify: `bun run check:i18n` and `task test:web` pass.
- [ ] 5. End to end with fake media devices in Chromium. Verify: `bun run e2e` passes a spec that records, uploads and places a clip.

## Test strategy

Service tests for the upload; Playwright with Chromium's fake device flags; Firefox and WebKit are reported as run or not run.

## Acceptance criteria

1. **Given** Chromium on localhost, **when** a person records 30 seconds, **then** an asset exists with a duration of 30 s plus or minus 1 s (`bun run e2e`).
2. **Given** a page on plain HTTP outside localhost, **when** the dialog opens, **then** recording is disabled and the reason is shown.
3. **Given** a chunk with a wrong checksum, out of order or over the limit, **when** it is sent, **then** it is refused with a code from SDD-000.
4. **Given** a tab closed mid-recording, **when** the server is checked, **then** the chunks received can be finished into a playable file.
5. **Given** an agent token, **when** an upload route is called, **then** the answer is `E_PRIVILEGED_SESSION`.
6. `task lint`, `task test` and `task test:web` pass.

## Risks and mitigation

- Browsers differ on codecs and system audio: stated in the dialog and in the user documentation.
- Disk abuse in network mode: per-actor limits.
- **STOP** if a rule would have to run in TypeScript, or if a new npm dependency seems necessary.

## Size

M: a recorder module and one upload contract.

## Priority

Should. It serves Docker and VPS users, but the desktop MVP stands without it.

## Hand-off for review

The commits, the browsers actually run, each criterion with its command and result.
