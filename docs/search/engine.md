# Video editor engine: what the research changes in our architecture

> Stage 1: Research. All sources accessed 2026-11-14.
> Labels: **FACT** (verified in the source), **INFERENCE** (deduction, with the reasoning), **OPINION** (the architect's judgment).
> Full evidence, with all citations: [`notes/engine-postmortems.md`](notes/engine-postmortems.md). Facts personally re-checked: [`notes/architect-verifications.md`](notes/architect-verifications.md).

## Index

1. [Summary of recommendations](#1-summary-of-recommendations)
2. [Time representation](#2-time-representation)
3. [Timeline data model](#3-timeline-data-model)
4. [Commands, undo/redo and multiple actors](#4-commands-undoredo-and-multiple-actors)
5. [Ingest: normalize before editing](#5-ingest-normalize-before-editing)
6. [Audio pipeline and A/V sync](#6-audio-pipeline-and-av-sync)
7. [Proxies, thumbnails, waveforms and cache](#7-proxies-thumbnails-waveforms-and-cache)
8. [Project format and migrations](#8-project-format-and-migrations)
9. [Gaps in this area](#9-gaps-in-this-area)

## 1. Summary of recommendations

| Topic | Recommendation (OPINION, justified below) | What this changes |
| --- | --- | --- |
| Time | Integers, never floats, inside the core. Seconds and timecode only at the edge (CLI/MCP/API/UI), converted by a single function that returns the adjusted value | Timeline schema, command catalog, all agent tools |
| Item position | Absolute position per item (`track` + `start` + `duration`), not a sequential list with gaps | Commands become idempotent and order-independent; ripple becomes a command, not a property of the model |
| Transition | Property of the clip (in/out), not its own item, in the MVP | Fewer cross-references; simpler undo |
| History | Ordered log of operations, authoritative server, last-writer-wins per property. No CRDT | Real-time protocol, audit per actor, undo per actor |
| Undo | Compensating operation emitted by the same actor; side effects (upload, proxy, export) stay outside the history | Command catalog must declare the inverse of each command |
| Ingest | Every imported video generates a normalized copy (CFR, rotation applied, SDR BT.709, short GOP) | Preview and export read the same normalized media; a whole class of bugs disappears |
| Audio | Mixing in 48 kHz samples; final file duration validated by ffprobe against the timeline | Export acceptance criterion |
| Project | Integer `schemaVersion`, chained migrations with a fixture per version, atomic saving, media referenced by content hash | SDD for the schema and persistence |

## 2. Time representation

### What the sources show

- **FACT.** WebCodecs defines timestamp as an integer number of microseconds. An integer microsecond cannot represent 1/30000 s or 1/48000 s without rounding. Source: <https://w3c.github.io/webcodecs/>.
- **FACT.** OpenTimelineIO uses `RationalTime` (value + rate), with `rescaled_to` to convert and comparisons that rescale. Source: <https://raw.githubusercontent.com/AcademySoftwareFoundation/OpenTimelineIO/main/src/opentime/rationalTime.h>.
- **FACT.** MLT (Shotcut, Kdenlive) expresses everything in whole frames, and the frame-to-second mapping depends on the project profile. Source: <https://www.mltframework.org/docs/mltxml/>.
- **FACT.** OpenCut replaced float seconds with an integer tick count (`TICKS_PER_SECOND = 120_000`) with a rational `FrameRate`, justified by float "accumulated rounding errors". Re-checked in the code (`rust/crates/time/src/media_time.rs`). Details in [`projects/opencut.md`](projects/opencut.md).
- **FACT.** The "flick" (1/705,600,000 s) divides 24, 25, 30, 60 Hz and the usual audio rates, but does not divide the NTSC rates (29.97; 23.976; 59.94). Source: <https://github.com/facebookarchive/flicks>.
- **FACT.** The mediabunny author on float: "you need to be really diligent and precise to make sure precision errors cannot creep in. For those cases, either convert to some integer coordinate system or use fuzzy epsilon compares". Source: <https://github.com/Vanilagy/mediabunny/issues/69>.

### Real bugs caused by float or inconsistent rounding

| Case | What happened | Source |
| --- | --- | --- |
| Remotion #12603 | Preview shows one frame behind the render. Preview and render chose the frame with different rounding rules | <https://github.com/remotion-dev/remotion/issues/12603> |
| Editly #49 | The same 90 s JSON renders 88.70 s at 10 fps and 88.92 s at 30 fps | <https://github.com/mifi/editly/issues/49> |
| MLT #69 | Audio one frame out of sync when the clip has an in-point | <https://github.com/mltframework/mlt/issues/69> |

### Options for our core

| Option | Pros | Cons |
| --- | --- | --- |
| A. Integer frames at the project's rational rate (`fps = {num, den}`) | No state exists off the grid; "which frame is time t" is trivial; it is what MLT and Remotion do; small, readable numbers in JSON | Changing the project fps rescales everything (with rounding); audio precision limited to one frame |
| B. Integer ticks on a fixed base (e.g., 240,000/s) | Changing fps loses nothing; 48 kHz audio fits exactly (5 ticks per sample); items with different fps coexist | An invalid state now exists (item off the frame grid), which needs an invariant and validation; large, opaque numbers in JSON |
| C. Rational per value (`{value, rate}` like OTIO) | Maximum generality; direct interchange with OTIO | Two numbers per time, comparison with rescaling, more surface for error; generality that a 30 s to 5 min editor does not use |
| D. Float seconds | Simple to write | It is the cause of the bugs in the table above. Discarded |

Note on the OpenCut base: 120,000 ticks/s divides 24, 25, 30, 60 and the NTSC rates, but **does not** divide 48,000 Hz (120,000 / 48,000 = 2.5). INFERENCE from the architect's arithmetic: whoever chooses ticks should use 240,000/s, which divides the same video rates and also 48 kHz.

**Recommendation (OPINION): option A.** Every time quantity on the timeline is an integer count of frames at the project rate, and the project rate is a rational (`30/1`, `30000/1001`). Reasons: (1) it rules out off-grid states by construction, which option B would require validating in every command; (2) the two use cases are videos of 30 s to 5 min, with the project fps decided at the start; (3) one-frame precision for audio cuts matches the reference editors. The position inside the source media uses the same unit, and choosing the source frame is a pure function with integer arithmetic (see section 5). Changing the fps of an existing project becomes an explicit command that rescales and reports what was rounded.

**Edge.** Agents and humans think in seconds and timecode. The conversion `seconds → frames` happens in a single function in the core, and each command's response returns the effective value ("requested 12.48 s, applied frame 374 = 12.467 s"). No layer (UI, CLI, MCP, API) does this math on its own.

## 3. Timeline data model

### Observed models

| Project | Time | Position | Transition | Source |
| --- | --- | --- | --- | --- |
| OpenTimelineIO | rational | sequence inside the track, with explicit `Gap` | own item between two neighbors | <https://opentimelineio.readthedocs.io/en/latest/tutorials/otio-timeline-structure.html> |
| MLT XML | frames | sequential playlist, with explicit `blank` | separate node that references two tracks | <https://www.mltframework.org/docs/mltxml/> |
| Shotstack (declarative JSON, commercial API) | float seconds | absolute `start` + `length` per clip; array order is the layer | property of the clip (`in`/`out`) | <https://shotstack.io/docs/guide/llms-full.txt> (partially read) |
| OpenCut classic | integer ticks | absolute `startTime` + `duration` per element | no transitions | [`projects/opencut.md`](projects/opencut.md) |

### What this implies (INFERENCE)

1. **Absolute position wins for an editor operated by agents.** In a sequential model (OTIO, MLT), moving a clip changes the position of every following clip, and a command only makes sense relative to the current order. In an absolute model, "move item `X` to frame 300" has the same effect no matter what else happened, which fits idempotency, last-writer-wins per property, and two actors editing at the same time. The "ripple" effect (closing the gap) becomes a command that emits several adjustments, not a property of the model.
2. **Transition as a clip property in the MVP.** Olive accumulates crash issues precisely in undo involving a transition between two clips (#1781, #2132, titles read). A transition that references two items must be rebuilt on undo. As an in/out property of the clip, it disappears along with the clip and creates no cross-reference. The cost is expressiveness: a cross-dissolve between two clips requires explicit overlap on different tracks or an overlap rule on the same track. Acceptable in a minimalist editor.
3. **Caption is a first-class item, not plain text.** OpenCut treats captions as a standalone `TextElement`, and a production product reported five points of friction because of it (issue #898, cited in [`projects/opencut.md`](projects/opencut.md)). For our use cases (mandatory captions, word-by-word style), the caption item must carry `words[]` with times and be editable as a block (single style for the whole track).
4. **Word times on the source media base.** If word times are tied to the transcribed media segment, the caption survives cuts and speed changes by recalculation. If they are in timeline time, every cut forces the caption to be rewritten. The open Remotion request (#12927, "caption retiming across cuts and speed changes", title read) shows this pain is real.
5. **Layer order by fractional index.** Figma orders children by a fraction between 0 and 1, which allows inserting between two neighbors without renumbering the others and reduces conflict between actors. Source: <https://www.figma.com/blog/how-figmas-multiplayer-technology-works/>.
6. **Nesting (composite clip) is out of the MVP.** It is enough that IDs are stable and the schema does not prevent adding it later.

## 4. Commands, undo/redo and multiple actors

### Compared patterns

| Pattern | Serializable | Human + agent in the same project | Audit | Undo per actor |
| --- | --- | --- | --- | --- |
| Command with in-memory inverse (classes) | no, if they are objects; this is the case of OpenCut classic | poor: the inverse assumes the state in which it was applied | weak | hard |
| Immutable snapshots | expensive | global undo, no notion of author | weak (no intent) | no |
| Patches (Immer, JSON Patch) | yes | medium | medium | same limitation as the inverse |
| Ordered log of operations (event sourcing) | yes: the log is the format | natural: each operation carries the actor | strong: the log is the trail | undo becomes a new compensating operation |
| CRDT (Yjs, Automerge) | binary, less readable | good for offline and P2P | medium | per origin |

### Evidence

- **FACT.** Figma: "the server is the ultimate authority on what the document looks like"; the server sets the order of events; conflicts are resolved by last-writer-wins per object property; edits to different properties do not conflict; the server rejects changes that would break the document. The team ruled out OT ("unnecessarily complex for our problem space") and did not implement a full CRDT, because CRDTs serve decentralized systems. On undo: undoing many steps, copying, and redoing back to the present should not change the document; undo and redo adjust each other's history. Source: <https://www.figma.com/blog/how-figmas-multiplayer-technology-works/>.
- **FACT.** tldraw sync keeps an authoritative copy per document and uses migrations and schema validation for clients on different versions. Source: <https://tldraw.dev/docs/sync>.
- **FACT.** OpenCut classic implemented undo as non-serializable TypeScript classes, with no MCP, CLI or API, and then went for a rewrite. Source: [`projects/opencut.md`](projects/opencut.md).
- **FACT.** Olive #1705: after deleting an audio clip and undoing, the old sound kept playing, even after saving and reopening. Source: <https://github.com/olive-editor/olive/issues/1705>.

### Conclusion (INFERENCE)

A timeline document is the same kind of thing as a Figma document: objects with discrete properties. The model "authoritative server + ordered log + last-writer-wins per property" is enough for one human and some agents in the same project, and it makes a CRDT unnecessary. The real risk is not fine-grained conflict resolution, but order and idempotency: each operation needs its own identifier, the actor, and the revision it was based on.

Implications for the specs:

1. Each command from the UI, CLI, MCP and API becomes a serializable operation, applied by a deterministic function `state' = apply(state, operation)`.
2. Undo is a compensating operation emitted by the same actor over the properties it changed. If another actor has already changed the same property afterwards, the undo of that property is discarded and this is reported.
3. Side effects (upload, proxy generation, transcription, exported file) do not enter the history. They are immutable artifacts addressed by hash. Undoing an import removes the reference from the timeline, not the file.
4. Everything the preview shows is derived from the state per revision. No preview cache is maintained by hand (lesson from Olive #1705).
5. Every command has a test: "apply, undo, redo returns an identical state".

## 5. Ingest: normalize before editing

The input videos for the two use cases come from phones and screen recordings, which are exactly the problematic sources.

| Source problem | Evidence | Consequence if ignored |
| --- | --- | --- |
| Variable frame rate (VFR) | **FACT.** Recordings with NVENC/AMF/QSV at "30 fps" have real intervals between 29.4 and 30.3 fps (OBS #13396, <https://github.com/obsproject/obs-studio/issues/13396>). Shotcut asks whether to convert when importing an iPhone `.mov` because it is VFR | Wrong frame selection, progressive loss of sync |
| Rotation and mirroring via metadata | **FACT.** iPhone video came out with the wrong rotation during conversion (Remotion #5509). Files can carry scale, translation and mirroring in the `mvhd` and `tkhd` matrices, not just rotation (mediabunny #492, <https://github.com/Vanilagy/mediabunny/issues/492>) | Video lying sideways or mirrored in one of the render paths |
| HDR (iPhone HLG) | **FACT.** An iPhone HLG clip converted to BT.709 came out "washed out" in Shotcut; the developers' answer: "converting HDR to SDR is inherently lossy, and there is more than one way to do it" (<https://forum.shotcut.org/t/iphone-hdr-washed-out/47286>). Chrome ignores the `colorSpace` of H.264 in macOS VideoToolbox and returns BT.709 (<https://issues.chromium.org/issues/532860177>) | Different colors between preview and export |
| Long GOP | **INFERENCE.** Exact seeking requires decoding from the previous keyframe up to the target frame. LosslessCut has lived with this for years (#126, #3210) | Slow and imprecise scrubbing |
| Negative timestamps and jumps | **FACT.** LosslessCut #2921: timestamp jumps when joining files (<https://github.com/mifi/lossless-cut/issues/2921>) | Gaps and freezes |

**Recommendation (OPINION).** Every imported video goes through an ingest step that produces **normalized media**: constant frame rate, display matrix already applied, SDR BT.709 with limited color range (with documented tone mapping when the source is HLG or PQ), short GOP and 48 kHz audio. Preview and export read that same normalized media. The original is preserved. This trades CPU time at import for an entire class of bugs that never comes to exist, and it is what gives meaning to the tolerance for divergence between preview and export.

The two points this recommendation left open were settled in ADR-0010: the original, the normalized copy and the proxy are all kept on disk, and the export reads the normalized media (one extra generation of compression, one code path), to be revisited only if a quality complaint justifies exporting from the original.

**Source frame selection.** With time in whole frames and the source at a constant frame rate, the source frame for timeline frame `t` is a pure function of integers (item position, in-point, speed as a rational, project fps and source fps). This function exists once and is used by both preview and export. It is the direct lesson of Remotion #12603.

## 6. Audio pipeline and A/V sync

- **FACT.** A 1 s MP4 reads 1.067 s because the AAC encoder adds priming that the file does not declare. WebCodecs does not report the priming, and the measured value varies by platform: 2112 samples (Chrome on macOS and WebKit), 0 (Chrome on Windows), 2048 (Chrome on Android). The fix in MP4 is the edit list. Source: <https://github.com/Vanilagy/mediabunny/issues/447>.
- **FACT.** The WebCodecs specification still does not expose priming and padding (issue open since 2023). Source: <https://github.com/w3c/webcodecs/issues/626>.
- **FACT.** In Remotion, cases of duration inflated by AAC end padding (#7099) and a 46.4 ms deviation in HLS audio (#12703) still remain.
- **FACT.** In Editly, audio goes out of sync when the source audio tracks have different lengths, because audio is concatenated per clip without filling silence up to the exact duration. Source: <https://github.com/mifi/editly/issues/327>.
- **FACT (weak).** No official YouTube, TikTok or Instagram page with a loudness target was found. The recurring value in secondary sources is -14 LUFS integrated. The existing Studio app already adopts -14 LUFS and -1.5 dBTP as a project rule ([`notes/existing-studio.md`](notes/existing-studio.md)).

Implications (INFERENCE):

1. Each audio item is rendered with exact duration in samples, with fill silence, before mixing.
2. Mixing happens at 48 kHz. The conversion from frames to samples is done once, at the edge of the mixer.
3. The export only counts as successful after the duration of the final file, measured by ffprobe, matches the timeline duration within a defined tolerance. Studio already has this check (`app/studio/qa.go`).
4. The audio preview is a path separate from the audio render (Remotion #11732: during the first 5 to 60 ms of each clip in sequence, the audio disappears in the preview, and the render is correct). The truth is the offline render; the preview needs its own tests, with hard cuts as a test case.
5. Loudness normalization is measured on the final render in two passes, not on the preview. Ducking is a command that generates volume keyframes, and therefore stays auditable and reversible.

## 7. Proxies, thumbnails, waveforms and cache

- **FACT.** The WebCodecs specification says codec resources "MAY be quickly exhausted and SHOULD be released immediately". Source: <https://w3c.github.io/webcodecs/>.
- **FACT.** A leak of about 3.3 MB per frame when reading samples, with RAM only stabilizing after a fix in the library and a report to Chromium. Source: <https://github.com/Vanilagy/mediabunny/issues/144>.
- **FACT.** A 20 min video export stopped at 5% without an error; at 40 min, "Array buffer allocation failed". The OpenCut export writes everything in memory. Source: <https://github.com/OpenCut-app/OpenCut/issues/914>.

Implications (INFERENCE):

1. Thumbnails and waveform peaks are computed once, at ingest, and persisted under the same key as the asset.
2. The key of any derived artifact is `content hash + version of the profile that generated it`. Invalidating means changing the key; an artifact is never altered in place.
3. In the browser, every `VideoFrame` has a deterministic `close()`, the frame cache is limited by bytes (not by count), and no path assembles the entire final file in memory.

## 8. Project format and migrations

- **FACT.** Olive: "Olive refuses to open a project file if too old"; the maintainer admits that "there hasn't been any effort in 0.2 to 'upgrade' projects when the format is bumped" and that it would be easier to redo the project than to migrate it. Sources: <https://github.com/olive-editor/olive/issues/1700>, <https://github.com/olive-editor/olive/issues/2229>.
- **FACT.** OpenCut classic keeps 31 additive storage migrations, with tests. Source: [`projects/opencut.md`](projects/opencut.md).

Recommendation (OPINION):

1. An integer `schemaVersion` at the top of the document, read before any other field.
2. Pure, chained migrations `vN → vN+1`, with one project fixture per version kept in the tests permanently.
3. Unknown fields are preserved on save, never discarded.
4. Atomic saving (write to a temporary file and rename) and a backup before migrating.
5. Media referenced by `{id, content hash, relative path, original name, probe metadata}`, to allow relinking by hash.
6. OpenTimelineIO serves as the interchange format (export and import), not as the native format.

## 9. Gaps in this area

- Schemas of Kdenlive, Olive, FCPXML, CMX3600 and Creatomate were not opened. The one for Shotstack was partially read.
- Engineering blogs of Clipchamp, CapCut, Canva, Descript, Kapwing and Veed were not covered by this area (the CapCut case on web.dev appears in [`projects/capcut.md`](projects/capcut.md)).
- No primary source for: official loudness targets of the platforms, the difference in color range (full and limited) with a specific issue, alpha channel, AV1, ProRes, AC3, PCM, offline media and determinism across hardware encoders.
- HLG to SDR tone mapping parameters in FFmpeg were not verified. Before fixing the ingest profile, a test with real iPhone videos is needed.
