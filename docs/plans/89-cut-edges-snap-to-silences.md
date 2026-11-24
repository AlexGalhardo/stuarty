# 89: Cut edges snap to silences

## Goal

When a creator deletes words from a transcript, the cut falls in the pause next to the words instead of inside a neighbouring word: a removed "um" no longer clips the start of the next word or leaves a sliver of the one it replaced.

## Context and related decisions

- Author decision D-135 (item 7): the measurement of epic 69 is accepted as it is, and a small epic follows that snaps the edges of a text cut to the nearest silences. An aligner (a new model) stays a decision for the author (D-133); this epic adds none.
- Epic 69 measured that a word inside a run of speech starts a median 160 to 240 ms from its audible onset (up to 600 ms at the 90th percentile), because whisper.cpp gives contiguous words and its post-processing can only correct a boundary where there is a pause (SDD-023 section 7, SDD-011 section 10.4). A cut whose edges are inside a run follows the whisper times, so a neighbouring word can lose up to that error. SDD-023 section 7 names this epic: "snapping the edges to the silences of the asset is the refinement".
- What exists today and is reused:
  - `transcript.delete_range` (`core/command/transcript_delete.go`, SDD-002 C.87, SDD-023 section 3) cuts the source range `R = [words[from].Start, words[to].End)` and takes the words through `resolved`, which the service fills in `resolveWords` (`service/resolve.go`).
  - The silence ranges of an asset come from the `silence` artifact of the ingest (SDD-011 section 9: thresholds -30, -35, -40 and -45 dB, minimum 150 ms). `resolveSilences` and `silenceFrames` in `service/resolve.go` already turn them into frames of the project rate for `clip.remove_silences` (`command.ResolvedSilences`, `command.SilenceRange`).
  - The labelled fixture and its floors: `core/transcript/testdata/`, `core/transcript/eval_test.go` (`TestLabelledFixture`), the report `docs/perf/2026-11-23-transcript-fillers.md`.
- The core stays pure (AGENTS.md section 3 rule 1): the silences reach the command as data in `resolved`, as for `clip.remove_silences`; the command starts no job and reads no file.
- ADR-0021 (whisper.cpp), SDD-011, SDD-023.

## User stories

- As a creator, I want a deleted sentence or filler to be cut in the silence beside it so that I do not hear a clipped word.
- As a creator, I want the review to tell me when a cut could not snap, so that I can nudge it myself.
- As an agent, I want the cut to be right without asking for frame-level edges, so that a word range is enough.

## Scope

- Modules: `core/command` (`transcript.delete_range`), `core/transcript` (the snap rule, a pure function), `service/` (`resolveWords` adds the silences), the transcript panel of the video editor (`web/resources/js/editor/inspector/panels/TranscriptPanel.tsx`, one line and one option), the generated command catalogue, SDD-002 (C.87), SDD-023, SDD-011 (section 10.8, the accuracy note), `docs/perf/` (a short addendum measured on the fixture), the user documentation of edit by text.
- Must not touch: `engine/transcribe/`, the ingest analysis (`engine/ingest/`), `renderer/`, `clip.remove_silences`.

## Out of scope

- Forced alignment or any new model; speaker diarization; changing which words a person deleted.
- Snapping `clip.remove_silences` or the filler review ranges (the detector keeps the whisper times; only the cut moves).
- New silence analysis: the snap uses the artifact the ingest already writes.

## Dependencies

69 (done). It touches `TranscriptPanel.tsx`, which epic 85 (video editor redesign) also rebuilds: the panel task (task 5) goes after 85 when 85 is in progress, and the other tasks do not depend on 85. It does not touch the modules of 61 to 68 or 70 to 72.

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | The panel sends the new option and shows whether each edge snapped |
| HTTP, CLI, MCP | Through the existing `edit` tool: `transcript.delete_range` gains an optional `snap` argument; no new tool, no new route |

## Core commands and permissions

No new command. `transcript.delete_range` (scope `edit`, undoable as one operation) gains:

| Argument | Meaning |
| --- | --- |
| `snap` (optional) | A length, the farthest an edge may move to reach a silence. `"0s"` turns the snap off. Default decided in task 1 (proposed: `"0.3s"`) |
| `resolved.silences` | Filled by the service, never by a caller, as `resolved.words` is: sorted, non-overlapping ranges in asset frames, half-open |

The result of the operation lists, for each cut, the edges it moved and by how many frames, in the `description` and the `diff` the service already returns, so that an agent and the panel can read it. Names are proposals until task 1 writes them to SDD-002 and SDD-023.

## Technical design

- **The rule** is `transcript.SnapEdges(r Range, silences []Silence, window int, words []Word, from, to int) Range`, pure and deterministic, in asset frames:
  - The **start edge** looks for a silence that contains the edge or whose nearest point is within `window` of it, and moves the edge to the point of that silence nearest to it (the edge is unchanged when it is already inside one). It never moves past the end of the word before `from` as the transcript gives it, so a silence the transcript contradicts is not trusted; and it never moves later than the first frame of word `from`.
  - The **end edge** is the mirror: toward a silence at or after the last word, never earlier than the end of word `to`, never past the start of word `to + 1`.
  - When two silences qualify the nearer wins; a tie takes the earlier one. When none qualifies, the edge stays at the whisper time and the result says so.
  - The snapped range always contains the words the person chose and never grows into speech the transcript shows: a snap can only remove silence the creator did not ask to keep, never a word they did not select.
- **The service** adds the silences of the asset at the default threshold (-35 dB, as `clip.remove_silences`) to `resolved` for `transcript.delete_range`. An asset with no `silence` artifact is not an error here: the silences are empty, the cut is the whisper cut, and the result says that no silence analysis was available (a transcript cut must keep working for an asset analysed before the artifact existed).
- **Speed and several items.** The snap happens once, in asset frames, before the range is turned into the local frames of each item (SDD-023 section 3), so every clip and its linked audio cut at the same snapped source range.
- **The panel** sends `snap` unchanged from the default, draws what the result says (which edges snapped), and computes no frame (SDD-012 section 1).
- **Measurement.** The accuracy claim is only made after it is measured: on the labelled fixture (it needs the `silence` artifact of the fixture audio, produced by the product's own ingest) the report gives, for cuts of one word inside a run, how often the unsnapped edge and the snapped edge land inside a neighbouring word, and the floors go into `eval_test.go` ten points under the measurement, as epic 69 did.

## Tasks

- [x] 1. Specify the snap first: SDD-002 C.87 (the argument, the result lines, the examples), SDD-023 sections 3 and 7, SDD-011 section 10.8; decide the default window and whether `snap` is on by default. Verify: `task lint:md` exits 0 and `task codegen` leaves no unexpected diff after the argument is added in task 3.
- [x] 2. The pure rule `transcript.SnapEdges` with table tests: an edge inside a silence, near one, in a run with none in the window, two candidates, a tie, the two guards (never into a chosen word, never into the next word), adjacent silences, the first and last word of the asset. Verify: `go test ./core/transcript/... -run Snap` passes.
- [x] 3. The argument and the `resolved.silences` member in `transcript.delete_range`, the golden cases of epic 69 unchanged with `snap` off, new golden cases with it on (speed 1, speed 3/2, a clip cut in pieces, a clip and its linked audio). Verify: `go test ./core/command/... -run 'TranscriptDelete'` passes and `task codegen` leaves no unexpected diff.
- [x] 4. The service fills the silences, degrades to the whisper cut when the artifact is missing, and writes the snapped edges in the description. Verify: `go test ./service/... -run 'Transcript|Resolve'` and `go test ./web/app/servicetest/... -run 'Transcript'` pass, including the case with no `silence` artifact.
- [x] 5. Measure on the labelled fixture and fix the floors. Verify: `go test ./core/transcript -run TestLabelledFixture -v` prints the unsnapped and the snapped numbers and passes with the new floors, and `docs/perf/` holds the addendum.
- [x] 6. The panel: the option and the line that says which edges snapped. Verify: `bun run typecheck`, `bun run check:thin-client`, `bun run check:i18n` and `task test:web` pass, and a Playwright spec deletes a sentence and shows the line (`bun run e2e` with the spec).
- [x] 7. Surfaces and agent documentation: the catalogue entry stays within budget and the user guide says what snapping does and does not do. Verify: `go test ./web/contract/...` and `go run ./internal/tools/surfacebudget` pass.

## Test strategy

Pure table tests for the rule; golden timelines for the command with the snap on and off (the cases of epic 69 stay byte-identical with it off); a service test with and without the artifact; the labelled fixture for the measurement; one Playwright spec. No FFmpeg run is needed beyond producing the fixture's `silence` artifact once, and the artifact is kept in `core/transcript/testdata/`.

## Acceptance criteria

1. **Given** a cut of one word inside a run whose neighbours have a pause of at least 150 ms within the window, **when** `transcript.delete_range` runs with the snap on, **then** each edge of the removed source range lies inside a silence of the artifact (`go test ./core/command/... -run TranscriptDeleteSnap`).
2. **Given** `snap` is `"0s"`, **when** the same cut runs, **then** the timeline is byte-identical to the result of epic 69 for every golden case of `TestTranscriptDeleteRangeOnHardTimelines` and `TestTranscriptDeleteRangeLosesTheRangeWithinOneFrame`.
3. **Given** a silence that overlaps a word the person did not select, **when** the edge would move into it, **then** the edge stops at the border of the chosen words (a table case of `SnapEdges`).
4. **Given** an asset with no `silence` artifact, **when** a cut runs, **then** it succeeds with the whisper cut and the result says that no silence analysis was available.
5. **Given** the labelled fixture, **when** `TestLabelledFixture` runs, **then** it prints the share of one-word cuts that clip a neighbouring word, unsnapped and snapped; snapped cuts clip a neighbour no more often than whisper cuts and leave fewer slivers of the removed word, on the labelled fixture; the floors it fails below are written in `docs/perf/`. (Amended in review under D-139: the rule only moves an edge outward, so the cut always contains the chosen words and cannot lower clipping; the measurement is equal clipping and fewer slivers, `docs/perf/2026-11-24-transcript-snap.md`. A real gain on clipping needs an aligner, which is the author's decision, D-133.)
6. Undo of a snapped cut restores the previous document byte for byte (an undo case in the golden test); the timeline revision does not change before a person confirms (unchanged from epic 69).
7. `task lint`, `task test`, `task test:web` pass; no new dependency; `go run ./internal/tools/surfacebudget` passes.

## Risks and mitigation

- A silence at -35 dB can be a quiet word (a whispered ending) that the transcript knows. Mitigation: the guards of the rule use the words of the transcript, and the window is small; the measurement counts cuts that clipped speech, not only cuts that landed in silence.
- The default changes the result of an existing command for every caller. Mitigation: task 1 decides it in the specification with the author's rule of keeping goldens stable (`snap` off reproduces epic 69 byte for byte), and the release notes name the change.
- **STOP** if the measurement in task 5 shows that the snap clips more speech than the whisper cut on the fixture: the rule is wrong, report the table and do not ship a default.
- **STOP** if a second silence analysis, a new model or an aligner turns out to be needed (a new dependency, D-133): report it for the author.
- **STOP** if the panel change collides with epic 85 in the same files: leave task 6 for after 85.

## Size

S: one pure rule, one argument, one service step and a line in the panel.

## Priority

Should. A cut that clips a word is the first thing a creator hears after deleting text; it makes epic 69 trustworthy at low cost.

## Hand-off for review

The commits; the specification diff (SDD-002 C.87, SDD-023, SDD-011); the unsnapped and snapped numbers of the fixture and the addendum in `docs/perf/`; each acceptance criterion with its command and result; which default was chosen and why; whether task 6 ran before or after epic 85.
