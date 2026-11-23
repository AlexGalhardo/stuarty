# 71: Caption styles, sensitive-data blur and studio voice

## Goal

A creator finishes a video in a few clicks: a captioned look from a library, private data blurred, and a voice that sounds clean and even.

## Context and related decisions

- Author decision (2026-11-23): caption styles and privacy blur are essential. The competitor is a reference only: styles here are original.
- `core/command/caption_transcript.go`, `core/command/filter.go` (`clip.set_blur`), `core/command/clip_adjust.go` (`clip.set_mask`), `core/command/audio_processors.go` (`audio.processor_add`), `engine/audio/denoise.go`, the `loudness_target` setting, ADR-0042.

## User stories

- As a creator, I want ready caption styles so that my shorts look finished.
- As a creator, I want to blur an email or key on screen so that I do not leak private data.
- As a creator, I want one action for clean voice so that I do not tune audio processors.
- As an agent, I want to apply these by name so that I can finish a video on request.

## Scope

- Modules: `core/` (named presets and commands), the inspector UI, SDD-002, user docs.
- Must not touch: `engine/transcribe/`, the FFmpeg policy, `capture/`.

## Out of scope

Automatic detection of private data, new audio models (DeepFilterNet needs a separate decision), sound packs.

## Dependencies

None from the capture epics. It can run side by side with them when modules do not intersect.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | Style gallery, blur tool, voice preset |
| HTTP, CLI, MCP | Through the existing `edit` tool |

## Core commands and permissions

Names are proposals until task 1 writes them to SDD-000 and SDD-002. All are undoable commands with scope `edit`.

| Command | Does |
| --- | --- |
| `caption.apply_style` | Applies a named style to a caption track |
| `clip.blur_region` | Adds a blurred rectangle with a time range, built on existing blur and mask |
| `audio.apply_voice_preset` | Adds the denoise, equalizer, compressor and loudness chain from existing processors |

## Technical design

- At least 12 original caption styles as data in the core, each resolving to existing style fields, and readable by agents through the catalogue.
- The blur renders in the export from the same frame plan as the preview. A strong default ensures the text cannot be read.
- The voice preset is a named combination of existing processors, editable after applying.

## Tasks

- [ ] 1. SDD-000 and SDD-002. Verify: `task lint:md` exits 0 and `task codegen` leaves no unexpected diff.
- [ ] 2. Caption styles and `caption.apply_style`. Verify: `go test ./core/...` with goldens per style.
- [ ] 3. `clip.blur_region`. Verify: `go test ./core/...` and an export golden.
- [ ] 4. `audio.apply_voice_preset`. Verify: `go test ./core/... ./engine/audio/...` and a measured loudness within 1 LU of the target.
- [ ] 5. Gallery, blur tool and voice action. Verify: `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n` and `task test:web` pass.
- [ ] 6. Surfaces and agent documentation. Verify: `go test ./web/...` and `go run ./internal/tools/surfacebudget` pass.

## Test strategy

Golden frames per style, export parity for blur, audio measurement on a fixture.

## Acceptance criteria

1. **Given** a caption track, **when** each style is applied, **then** the frame matches its golden and undo restores the previous style.
2. **Given** a blurred region over text, **when** exported, **then** the region's pixels differ from the source and the time range is exact to the frame.
3. **Given** a noisy voice fixture, **when** the preset runs, **then** integrated loudness is within 1 LU of the target.
4. No style name, text or visual copies a competitor's (reviewed by the specialist).
5. `task lint`, `task test`, `task test:web` pass; no new dependency.

## Risks and mitigation

- A weak blur can be reversed: strong default and a minimum.
- **STOP** if a style needs a font with an unclear licence or a new renderer feature.

## Size

M: presets over existing commands, plus one UI gallery.

## Priority

Should. High visible value at low risk.

## Hand-off for review

The commits, the golden images, the loudness measurement, each criterion with its command and result.
