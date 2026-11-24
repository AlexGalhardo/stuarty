# SDD-023: Edit by text, filler words and repetitions

- Status: Draft (2026-11-23). Written by epic 69 before its code.
- Depends on: ADR-0002, ADR-0007, ADR-0008, ADR-0021 (whisper.cpp, WhisperX rejected), ADR-0031, SDD-000 (names), SDD-002 (C.27, C.40, C.41, C.87), SDD-003 (the surfaces), SDD-007 (untrusted text), SDD-011 section 10 (the transcript), SDD-012 (thin client).
- Implements: author decision of 2026-11-23 (edit by text is essential; removals are reviewed first; AI is local by default) and D-133 (WhisperX is not shipped; whisper.cpp stays, its word timing is measured first).
- Owner area: epic 69 (`core/transcript/`, `core/command/transcript_delete.go`, `service/transcript*.go`, the `transcript` adapters of `web/app/`, the transcript panel of the video editor).

## Contents

1. Overview
2. Words and their timeline ranges
3. Deleting a word range
4. Detecting fillers and repetitions
5. Operations, scopes and surfaces
6. The review before removal
7. Measured accuracy and thresholds
8. Errors and limits
9. Tests

## 1. Overview

A creator edits a talking video by editing its transcript: deleting a sentence cuts the video there, and the filler words ("um", "uh", "ééé") and repetitions ("we should we should") that the product detects are reviewed by the creator before anything is removed.

Four rules carry the design:

1. **The core decides every cut.** The UI, the CLI and the agent send a word range; `core/` turns it into a ripple delete of the timeline. The UI computes no frame (rule 2 of AGENTS.md; SDD-012 section 1).
2. **A detection removes nothing.** `TranscriptDetectFillers` is a `read` operation that returns suggestions. The timeline revision changes only when a person confirms and an edit batch is sent. An agent proposes the same way, through the review flow of the approval queue (epic 64); until that epic is done an agent has no proposal path and cuts only through the `edit` tool with the person's own scope.
3. **No model is added.** Detection is a rule in the core over the words whisper.cpp already produced (a per-language list of hesitations and a window for repeated sequences). Forced alignment with a new model, speaker diarization and overdub are out of scope.
4. **Word indices are those of the transcript.** The `index` of a word is its position in the `words` array of the newest `transcript` artifact of the asset (SDD-011 section 10.5). Cutting does not change the transcript, so a batch of cuts can be written with the indices of the view it was drawn from.

## 2. Words and their timeline ranges

`core/transcript` is a pure package (no I/O, imports `core/timeline` only). Its types:

| Type | Fields |
| --- | --- |
| `Word` | `Text` (a word, no whitespace), `Start`, `End` (asset frames, half-open) |
| `Range` | `ItemID`, `TrackID`, `Start`, `End` (timeline frames, half-open) |
| `WordRanges` | `Index`, `Word`, `Ranges` (zero or more `Range`, ordered by `Start` then `ItemID`) |

`Map(doc, assetID, words)` is a pure function of the document and the transcript, recomputed after every edit (it is never stored). Its rule is the rule of C.40 postcondition 1 for every item that shows the asset:

1. The eligible items are the `clip` and `audio` items of the document whose `asset` is `assetID`, whose speed is constant and which are not reversed. An item with a `speed_curve` or reversed cannot map a frame of the timeline to a frame of the source (C.27, SDD-002 A item 7); it is listed in `Skipped` and no word is placed on it.
2. For an item `it` with source in `si`, speed `num/den` and visible source range `S = [si, si + ceil(duration x num / den))`, a word is placed on it when `word.Start` is in `S` (a word that starts inside the visible range, as in C.40). Its timeline range is `start = it.Start + ceil((word.Start - si) x den / num)` and `end = it.Start + ceil((min(word.End, srcEnd) - si) x den / num)`, with `end >= start + 1` and `end <= item end`.
3. A word placed on no item has an empty `Ranges`: it was cut (or lies outside every item) and the panel shows it struck through. A word placed on several items (the same part used twice) has one range per item.

The rule 2 formula is the one `mapTranscript` of `caption.from_transcript` already uses (that function also moves a start that would overlap the previous word, which cannot happen to words that the service already made disjoint in frames), so the captions and the transcript panel agree on every frame; a golden test pins the worked example of C.41 for both.

## 3. Deleting a word range

`transcript.delete_range` (SDD-002 C.87, scope `edit`) takes an `asset_id`, a `from_word` and a `to_word` (inclusive indices) and removes from the timeline the media of those words, closing the gap, as one undoable operation.

The source range to remove is `R = [words[from_word].Start, words[to_word].End)` in asset frames. For **every** eligible item of the asset (a clip and its linked audio, a clip used twice, a clip already cut into pieces) whose visible source range `S` meets `R`:

1. The cut is decided in the item's own frames. Local frame `k` shows source frame `floor(k x speed)`, so the frames that show `R` are `[ceil(a / speed), ceil(b / speed))`, with `a` and `b` the ends of `R` relative to `si`, clamped to the item (a range between two shown frames removes one frame). The item keeps the frames before and after them (an empty side is dropped), as one or two pieces: the piece after the cut has its `source_in` at `R.end`. The first piece keeps the id, the transitions-in and the fade-in, the last keeps the transitions-out and the fade-out, new pieces take `new_ids` or generated ids, and captions linked to the item are linked to the piece that holds their first word.
2. The pieces close up from the item's start and the later items of the **same track** move earlier by the removed length (ripple `track`). Items of other tracks do not move, exactly as in `clip.remove_silences`; a person who wants the whole timeline to follow cuts the linked tracks together, which this command does because it acts on every item of the asset.
3. An item wholly inside `R` is deleted, and later items of its track close the gap.
4. Items are processed from the latest `start` to the earliest on each track, so the ripple of one never moves an item not yet processed.

Each item loses the part of `R` it shows, converted to timeline frames, within one frame, at any constant speed: at speed 1 exactly, and at speed 3/2 a range of 0.67 frames removes one frame. At a speed above 1 the piece after the cut can read up to two timeline frames less of the source at its end, so a word that starts in those last frames of the item can lose its range. Speech that was spoken before `R.start` and after `R.end` is untouched. Captions already made keep their words; `caption.retime_from_clip` re-syncs them (the panel offers it after a cut).

## 4. Detecting fillers and repetitions

`transcript.Detect(words, options)` is pure and deterministic over the words of the transcript in milliseconds (the artifact's own times), so it needs no frame rate.

**Normalisation.** A word is lowercased and stripped of punctuation and symbols (letters, digits and inner apostrophes stay). Words are compared in this form.

**Fillers.** A per-language rule, chosen by the `language` of the transcript (`en`, `pt`; an unlisted language gets no filler rule and only repetitions). A word is a **strong** filler when its normalised form matches a hesitation pattern, and a **weak** one when it matches a discourse marker. Strong are removed first in the panel; weak are shown unchecked.

| Language | Strong (patterns on the normalised form) | Weak (exact words and phrases) |
| --- | --- | --- |
| `en` | `um`, `umm`, `uh`, `uhh`, `uhm`, `er`, `erm`, `hm`, `hmm`, `mm`, `mmm`, `mhm` (pattern `u+m+`, `u+h+m*`, `e+r+m*`, `h+m+`, `m{2,}`, `m+h+m+`) | `you know`, `i mean`, `ah` |
| `pt` | `hum`, `hmm`, `ahn`, `ãh`, `hã`, `éh`, a lengthened `é` of two or more letters (`éé`, `ééé`), `uh`, `uhm` | `né`, `tipo` |

A plain `é` (the verb) and a plain `um` in Portuguese (the article) are never fillers.

**Repetitions.** For a sequence length `n` from 4 down to 1: when the `n` normalised words at `i` equal the `n` words at `i + n` and the gap between the end of the first copy and the start of the second is at most `RepeatWindowMS` (default 700), the first copy `[i, i + n - 1]` is a repetition and the later copy is kept (the creator keeps the fluent take). Longer sequences are tried first and a word is part of at most one suggestion; fillers claim their words before repetitions do. A single word repeated is **weak** when it is an intensifier or an answer (`very`, `really`, `so`, `no`, `yes`, `bye`; `muito`, `não`, `sim`, `tchau`), strong otherwise; a sequence of two or more words is strong.

**Only what the timeline shows.** The service detects over the words that have a range on the timeline (`DetectShown`), with their transcript indices: a word already cut is not suggested again, so a second review cannot fail on it, and two copies that a cut made adjacent are a repetition.

**Result.** A `Suggestion` has `kind` (`filler` or `repetition`), `strength` (`strong` or `weak`), `from_word`, `to_word`, `text` (the words, untrusted text), and `start_ms`, `end_ms` (the span of the words). Suggestions are ordered by `from_word` and do not overlap. The same input gives the same output, byte for byte.

## 5. Operations, scopes and surfaces

| Operation | Scope | HTTP | CLI | MCP |
| --- | --- | --- | --- | --- |
| `TranscriptView` | `read` | `GET /projects/{id}/transcript?asset_id=` | `transcript:show` | `media_inspect` with the section `words` |
| `TranscriptDetectFillers` | `read` | `GET /projects/{id}/transcript/fillers?asset_id=` | `transcript:fillers` | `media_inspect` with the section `fillers` |
| `transcript.delete_range` | `edit` | `POST /projects/{id}/edit` | `edit` | `edit` |

`TranscriptView` returns `{revision, asset_id, language, words: [{index, text, start, end, ranges: [{item_id, track_id, start, end}]}], skipped_items}`, where `revision` is the revision of the timeline the ranges were computed against, `start` and `end` are asset frames and `ranges` timeline frames. `TranscriptDetectFillers` returns `{revision, asset_id, language, suggestions, rule, filler_rules}`: `rule` says in English what was checked, for an agent, and `filler_rules` says whether the language has a filler list, so that the panel says it in the language of the person. Neither starts a job: with no transcript they fail with `E_INVALID_STATE` and a hint that names `job_start` of kind `transcribe`, like the edits that need analysis (SDD-002 A.9). The text of a word is untrusted text (SDD-007 section 8). On the agent surfaces (`media_inspect` and the CLI) it is escaped, capped (80 characters a word, 200 a suggestion) and listed in an untrusted block with its index; the HTTP read keeps the cleaned text for the panel, which renders it with `UntrustedText`. `media_inspect` returns one page of the words or the suggestions within the budget of SDD-003 section 3.10: `cursor` pages the transcript when that section is asked for, else the words, else the fillers, and a section that does not own the cursor says that it was cut. Both read the transcript and the document under one snapshot; no connection is opened.

## 6. The review before removal

The transcript panel of the video editor is a thin client (SDD-012 section 1): it draws the words of `TranscriptView`, strikes the words with no range, and sends commands.

- Selecting words and pressing delete sends one `transcript.delete_range` for the selected indices. The panel reloads the view from the new revision; undo restores the words and the media.
- "Find fillers" calls `TranscriptDetectFillers` and lists the suggestions with the check boxes: strong checked, weak unchecked. With no suggestion it says what was checked (the language and the rule), not that the recording has no fillers. The timeline revision does not change until "Remove selected", which sends one edit batch with one `transcript.delete_range` per checked suggestion, so one undo restores all of them.
- The panel computes no frame, length or word match.

## 7. Measured accuracy and thresholds

Whisper can drop or smooth fillers, so what the pinned whisper.cpp (`v1.9.2`, model `small`) does with a spoken filler was measured before it was promised, on a labelled English and Portuguese speech fixture. The method, the numbers and the thresholds are in `docs/perf/2026-11-23-transcript-fillers.md` (SDD-011 section 10.8); the tests of the detector read the labelled transcripts of `core/transcript/testdata/` and fail below the floors. In short:

- **Repetitions** are found in both languages (recall 1.00 and 0.83). **English hesitations** that whisper writes (`um`, `hmm`, `er`) are found; one it drops (`uh`) cannot be. The filler recall is 0.69 in English.
- **Portuguese hesitations** are mostly written by whisper as the words `um` and `é`, which the rule must not flag; the filler recall is 0.06. The panel therefore tells the person what was checked and never says "no fillers" (section 6).
- A word that starts a sentence has its onset within about 25 ms of the audible onset (the post-processing of SDD-011 section 10.4 snaps it to the end of the pause); a word inside a run is a median 160 to 240 ms off. A cut whose edges are inside a run follows the whisper times, so the review shows the range and an edge may clip a neighbour or leave part of the word, by as much as that error (hundreds of milliseconds at worst). Snapping the edges to the silences of the asset is the refinement, not part of this epic.
- The fixture is synthetic: it bounds what the rules and the model do with clean, isolated hesitations and does not replace recordings of people.

## 8. Errors and limits

Codes are those of SDD-000: `E_VALIDATION` (an index out of range, `to_word < from_word`), `E_NOT_FOUND` (asset), `E_TYPE_MISMATCH` (an asset with no audio), `E_INVALID_STATE` (no transcript, no item shows the range, an item with a speed curve or reversed), `E_LIMIT` (500 items). A suggestion list has at most 2,000 entries. `transcript.delete_range` creates at most one new item per eligible item.

## 9. Tests

- Golden timelines for the mapping (one clip, a clip with a changed speed, several clips of one asset, a clip already cut, an item with a curve) and for `transcript.delete_range` (C.87), including the check that the words a delete removes have no range afterwards and every other word keeps its place.
- A labelled fixture of transcripts (`core/transcript/testdata/`) with the precision and recall of the detector, and the generator of the speech (`gen-speech.ps1`).
- The service tests of both reads, including that detection does not change the timeline revision.
- The parity test of SDD-003 section 7.1 and the surface budget.
- A Playwright spec that deletes a sentence in the panel and undoes it.
