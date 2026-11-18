# 35: Export presets, batch queue, H.265 and AV1

## Goal

A creator picks "YouTube Shorts" or "Instagram Reel" instead of guessing bitrates, queues every version of a video in one go, and gets smaller files where their machine can encode H.265 or AV1.

## Context and related decisions

- ADR-0042, ADR-0011 (only LGPL FFmpeg; no GPL encoder in anything we publish), ADR-0018 (jobs).
- Author decision D-067 (presets per platform, a batch queue, H.265 and AV1 through system and hardware encoders; no mastering formats). PRD-001 RF-087, RF-088, RF-089, NFR-043.
- SDD-004, SDD-011 section 8 (FFmpeg discovery and licence classification). Research note `web/02-redes-sociais` (each network's upload limits).
- Absorbs the platform limits of the earlier pilot plan.
- Open product issues this epic closes (their text comes in the briefing): VV-219 (an export fails by deadline when several exports and ingests run together), VV-220 (the export dialog refuses by concurrency and neither retries nor queues), VV-223 (the export dialog lacks PNG, AAC and a range, which the CLI and the API have).

## Scope

- Modules: `core/export/` (new, or the existing pure export rules: presets, validation, the batch), `engine/ffmpeg/` (encoder capabilities), `engine/export/` (the new codecs and verification), `service/` (batch operations), `web/app/` (controllers, CLI, MCP), the export dialog in `web/resources/js/editor/`, SDD-004, SDD-002, SDD-000.

## Out of scope

- ProRes, DNxHR, image sequences, HDR output, multi-pass tuning controls.
- Publishing (epics 18 to 22): a preset only guarantees that a file fits a network's rules.
- The own FFmpeg build (epic 46).

## Dependencies

03.

## Technical design

- **Presets as data** (RF-087): each preset has an id, the network and placement, container, codec preference order, resolution and aspect ratio, frame rate ceiling, bitrate or quality target, audio settings, maximum duration and file size, and the **source page and date** of those limits. Presets live in the core, are listed by `ExportPresetList` and described for agents. A preset whose limits were not read on an official page is not shipped.
- **Validation before encoding**: `ExportCheck(timeline, preset)` reports what does not fit (duration, aspect ratio, estimated size) with the exact limit, so an agent can fix the edit first. After encoding, the existing post-export verification also checks the preset's limits.
- **Codecs** (RF-089): `engine/ffmpeg` already classifies a build and refuses GPL encoders. The allow-list gains HEVC and AV1 encoders that are hardware (the vendor encoders of the three systems) or permissively licensed software; `libx265` and any GPL encoder stay refused even when present. A codec is offered only when discovery finds an allowed encoder that passes a one-second test encode; otherwise the preset falls back to H.264 and says so in the result.
- **Batch** (RF-088): `ExportBatch` takes a list of (timeline, preset or settings, range) and creates one parent job with child export jobs, run within the existing resource ceilings (one encode at a time by default). The parent reports per-child state; cancelling the parent cancels the rest; a failed child does not stop the others.
- **Outputs**: each finished file is recorded with its preset and verification result, and as an `output` item of the project when epic 06 is merged.
- **UI**: the export dialog lists presets grouped by network with their limits and source date, shows `ExportCheck` findings, allows several presets at once, and shows the batch progress.

## Tasks

- [ ] 1. Read the official upload specifications of YouTube Shorts, Instagram Reels, Facebook Reels, TikTok and X again; write the presets table with sources and dates into SDD-004; amend SDD-002 and SDD-000. Verify: `task lint:md` exits 0.
- [ ] 2. Implement presets and `ExportCheck` in the core with table tests. Verify: `go test ./core/... -run "TestExportPreset|TestExportCheck"` passes.
- [ ] 3. Extend encoder capabilities and the allow-list for HEVC and AV1, with the test encode. Verify: `go test ./engine/ffmpeg/... -run "TestEncoderAllowList|TestCapability"` passes, including a build that offers only a GPL encoder (refused).
- [ ] 4. Implement HEVC and AV1 export and extend the post-export verification. Verify: `go test ./engine/export/... -run "TestHEVC|TestAV1"` passes where an allowed encoder exists and skips with a stated reason where none does.
- [ ] 5. Implement `ExportBatch` with parent and child jobs. Verify: `go test ./service/... -run TestExportBatch` passes (partial failure, cancel, ceilings).
- [ ] 6. Expose on the three surfaces and regenerate documents. Verify: `go test ./web/contract/...` and `task generate:check` pass.
- [ ] 7. Rebuild the export dialog, in both languages. Verify: `task test:web` and `web/e2e/specs/export-presets-batch.spec.ts` pass.
- [ ] 8. Record on the author's machine which encoders were found and one export per codec. Verify: the list and the verification outputs are in the hand-off.

## Test strategy

- Unit: every preset's limits; `ExportCheck` at each boundary.
- Capability: fake `ffmpeg -encoders` outputs for each case (hardware only, permissive software, GPL only, none).
- Integration: batch state machine; verification of real encodes where the machine allows.
- End to end: choose two presets, run the batch, download both.

## Acceptance criteria

1. `ExportPresetList` returns presets for the five placements, each with its limits, source and date.
2. `ExportCheck` reports a timeline that is too long for a preset with the exact limit, before any encoding.
3. On a machine with an allowed HEVC or AV1 encoder the export uses it and passes verification; on a machine without one the export falls back to H.264 and says so.
4. A GPL encoder present in the user's FFmpeg is never used.
5. A batch of three exports with one failing ends with two files and one actionable error.
6. `go test ./core/... ./engine/ffmpeg/... ./engine/export/... ./service/... ./web/contract/...`, `task test:web` and `task generate:check` pass.

## Risks and mitigation

- **Hardware encoders behave differently per driver.** Mitigation: the test encode at discovery; fall back and report.
- **Network limits change.** Mitigation: source and date on every preset; the same table feeds the connectors' validation.
- **STOP** if no permissive or hardware encoder path exists for a codec on Windows: report and ship that codec as unavailable.

## Size

M

## Hand-off for review

The diff of `epic/35-export-presets-batch-and-codecs`; the presets table with sources; the encoders found on the author's machine; the outputs of the acceptance commands.
