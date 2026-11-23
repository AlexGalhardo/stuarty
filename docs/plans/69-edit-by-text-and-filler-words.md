# 69: Edit by text, filler words and repetitions

## Goal

A creator edits a talking video by editing its transcript: deleting a sentence cuts the video, and suggested filler words and repetitions are reviewed before anything is removed.

## Context and related decisions

- Author decision (2026-11-23): edit by text is essential; removals are reviewed first; AI is local by default.
- ADR-0021 (whisper.cpp; WhisperX rejected for its Python runtime), SDD-011 section 10.2.
- `engine/transcribe/whisper.go` (word timestamps), `core/command/caption_transcript.go`, `core/command/caption_words.go`, `core/command/clip_silence.go`.

## User stories

- As a creator, I want to delete a sentence in the transcript so that the video is cut there.
- As a creator, I want filler words and repetitions flagged so that I can choose what to remove.
- As an agent, I want to propose cuts by word range so that the person can review them.

## Scope

- Modules: `core/` (a transcript view, mapping from words to timeline ranges, detectors, commands), `service/`, the adapters, a transcript panel in the video editor, SDD-002, SDD-011, SDD-023.
- Must not touch: `engine/transcribe/` beyond reading its output, `renderer/`.

## Out of scope

Forced alignment with a new model, speaker diarization, overdub.

## Dependencies

The transcription that exists today. Epic 64 is required only for the agent's proposals through the review flow. **It does not depend on capture**, so it can run while 61 to 66 proceed if the modules do not intersect.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | Transcript panel |
| HTTP, CLI, MCP | Through the existing `edit` tool for commands, and existing read tools for the transcript view |

## Core commands and permissions

Names are proposals until task 1 writes them to SDD-000 and SDD-002.

| Command or operation | Scope | Does |
| --- | --- | --- |
| `TranscriptView` | `read` | Words with their timeline ranges |
| `transcript.delete_range` | `edit` | Ripple-deletes the timeline under a word range, as one batch |
| `TranscriptDetectFillers` | `read` | Returns suggestions and changes nothing |

## Technical design

- The word-to-range mapping is a pure function of the document and transcript, recalculated after each edit.
- **Filler detection** is a rule in the core: a per-language list, plus repeated word sequences within a short window. No model is added.
- Whisper can drop or smooth filler words, so word-level accuracy is measured in task 2 before it is promised.
- The UI only sends a word range; the core computes the cut.

## Tasks

- [ ] 1. SDD-000, SDD-002, SDD-011 and SDD-023. Verify: `task lint:md` exits 0 and `task codegen` leaves no unexpected diff.
- [ ] 2. Measure word timing and filler recall of the pinned whisper.cpp on a labelled pt-BR and English fixture. Verify: a report with precision and recall in `docs/perf/`.
- [ ] 3. The word-to-range mapping, including multi-clip timelines and speed changes. Verify: `go test ./core/...` passes.
- [ ] 4. `transcript.delete_range`, undoable. Verify: `go test ./core/...` passes.
- [ ] 5. Filler and repetition detection. Verify: `go test ./core/...` on the labelled fixture.
- [ ] 6. Adapters with parity. Verify: `go test ./web/...` and `go run ./internal/tools/surfacebudget` pass.
- [ ] 7. The transcript panel with review before removal. Verify: `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n`, `task test:web` and `bun run e2e` pass.

## Test strategy

Golden timelines for the mapping; a labelled audio fixture; a Playwright spec for deleting a sentence.

## Acceptance criteria

1. **Given** a transcribed clip, **when** a sentence is deleted, **then** the timeline loses exactly that range within one frame, and undo restores it.
2. **Given** a clip with a changed speed or several cuts, **when** a word range is deleted, **then** the right media range is removed (golden test).
3. **Given** detected fillers, **when** detection runs, **then** the timeline revision does not change until a person confirms.
4. **Given** the labelled fixture, **when** detection runs, **then** precision and recall are reported and meet the thresholds fixed in task 2.
5. `task lint`, `task test`, `task test:web` pass; no new dependency.

## Risks and mitigation

- Whisper may omit fillers: measured first.
- **STOP** if recall is too low to be useful: adding an aligner is a new dependency and the author's decision.

## Size

L: a precise mapping plus a new editor panel.

## Priority

Must. It removes the most repeated work in talking videos.

## Hand-off for review

The commits, the measurement report, each criterion with its command and result.
