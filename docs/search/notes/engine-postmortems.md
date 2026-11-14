# Non-linear editor engine and post-mortems

> **Status of this file.** Research notes delivered by a research agent on 2026-11-14 and reviewed by the architect. The opinions and recommendations in this file predate the author's decisions (`../../brainstorming.md`, D-001 to D-016) and the ADRs (`../../adrs/`). Where they differ, the ADRs prevail. Facts rechecked by the architect are listed in [`architect-verifications.md`](architect-verifications.md).

Conventions. All URLs below were opened on **2026-11-14** ("accessed 2026-11-14"). Labels: **FACT** (seen in the open source), **INFERENCE** (my reasoning) and **OPINION**. The **[training]** label indicates training knowledge I did not verify today; treat it as a hypothesis. What I could not open or find is stated literally in "Gaps".

The issue names of Remotion and mediabunny are from 2026 and the numbering is high; the numbers were read via `gh issue view`.

## Index

- Part 1: Engine
  - 1a Time representation
  - 1b Timeline model (schema comparison)
  - 1c Undo/redo, commands, sync and collaboration
  - 1d Decode, composition and encode pipeline (A/V, seek, VFR, rotation, HDR, color, audio)
  - 1e Proxies, thumbnails, waveforms, cache
  - 1f Project format, migrations, media
- Part 2: Post-mortems by theme (P1 to P16)
- The 12 lessons that most change the architecture
- Gaps

---

# PART 1: ENGINE

## 1a. Time representation

**What the sources show**

- **FACT** WebCodecs defines timestamp as "An integer value, in microseconds" for VideoFrame, AudioData and chunks (https://w3c.github.io/webcodecs/, accessed 2026-11-14). Integer microseconds do not represent 1/30000 s or 1/48000 s without a rounding error.
- **FACT** OTIO `RationalTime` is "a measure of time defined by a value and rate". `rescaled_to` converts from rate. The `==` rescales the right side to the left's rate, and there is a separate `strictly_equal` and `almost_equal(delta)`. Time is invalid if value or rate is NaN (https://raw.githubusercontent.com/AcademySoftwareFoundation/OpenTimelineIO/main/src/opentime/rationalTime.h, accessed 2026-11-14). The tutorial doc I opened does not explain why the rational form is used (the time-ranges page does not cover it). That "why" stays as INFERENCE: value/rate keep the time in the media's native unit, with no lossy conversion before comparison.
- **FACT** The "flick" (Facebook/Oculus) is 1/705600000 s and divides exactly 24, 25, 30, 48, 50, 60, 90, 100, 120 Hz and the audio rates 8, 16, 22.05, 24, 32, 44.1, 48, 88.2, 96 and 192 kHz. The NTSC rates (x1000/1001) are approximations, with no exact division (https://github.com/facebookarchive/flicks, accessed 2026-11-14). In other words: flicks handle 24/25/30/60 and 48 kHz, **not** 23.976/29.97/59.94.
- **FACT (secondary source)** Premiere uses 254016000000 ticks/s. 48 kHz gives 5292000 ticks per audio sample. The UI rounds to frames, and the API math is in ticks (Adobe community threads, e.g.: https://community.adobe.com/questions-729/getting-duration-out-of-pproticksin-pproticksout-1368450, accessed 2026-11-14). One thread shows a duration mismatch between ticks and frames with no answer in the excerpt I saw. In Premiere's XML, `<ntsc>TRUE</ntsc>` with timebase 24 means 23.976 (same set of threads).
- **FACT** MLT expresses everything as absolute frame counts, with inclusive `in`/`out`, and the frame-to-seconds mapping depends on the project's "profile" (https://www.mltframework.org/docs/mltxml/, accessed 2026-11-14).
- **FACT** Author of mediabunny on float: "It is possible to do with floats, but you need to be really diligent and precise to make sure precision errors cannot creep in. For those cases, either convert to some integer coordinate system or use fuzzy epsilon compares." The issue asked for rational timing (bigint timeScale/timeValue) on the EncodedPacket and was closed (https://github.com/Vanilagy/mediabunny/issues/69, accessed 2026-11-14).

**Real errors caused by float/rounding**

- **FACT** Remotion #12603 (https://github.com/remotion-dev/remotion/issues/12603): the preview shows 1 frame behind the render in WebM. Cause: the render uses `roundTo4Digits` plus a 1 ms tolerance to pick the frame, and the preview pipeline uses another comparison rule, so the two land on different frames when the timestamp in ms rounds to the opposite side of t. Lesson: **a single function for "which frame belongs to time t"**, shared by preview and export.
- **FACT** Editly #49 (https://github.com/mifi/editly/issues/49): output duration changes with FPS (90 s requested gives 88.70 s at 10 fps and 88.92 s at 30 fps, same JSON). Cause not identified in the text I read. INFERENCE: rounding of the duration to frames per clip and loss of duration at the transition.
- **FACT** Remotion #7099 (https://github.com/remotion-dev/remotion/issues/7099): AAC padding increases the MP4 duration by ~0.064 to 0.08 s because of the 1024-sample frame (see 1d).
- **FACT** MLT #69 (https://github.com/mltframework/mlt/issues/69, from 2015): audio out of sync by 1 frame with a clip in-point. The maintainer says they did not find how to get the exact number of samples to discard beyond `(our_time - stream_time) * stream_sample_rate`.

**How to handle 29.97/23.976, different fps and audio in samples**

- **INFERENCE (recommendation)**: timeline time as an **exact rational** `{num, den}` in integers (BigInt or an int64 pair). The project has a rational frame rate (30000/1001) and editing time is quantized to whole frames at that rate. For audio, convert to samples (48000) once, at the edge of the mixer. Flicks do not work as a single unit because 29.97 does not divide. Premiere's ticks (254016000000) also do not divide 1001 exactly [training: 254016000000 = 2^10·3^4·5^3·... factorization not confirmed].
- **INFERENCE**: media with a different fps from the project: the clip stores `source_in` in the **media's timebase** (rational), and source frame selection is a deterministic `floor`/`round` of `(t - clip_start) * speed + source_in`. Duplicate or drop frames, do not resample motion. Evidence of the pain: LosslessCut #455 shows 29.97/30/26.1 fps with different tbn (14625, 468k, 515700) freezing the second clip on merge (https://github.com/mifi/lossless-cut/issues/455, accessed 2026-11-14), and #2338 (merge fails due to framerate difference).
- **FACT** LosslessCut #2921 (timestamp jumps on merge): the dev suggests `-avoid_negative_ts make_non_negative` as a partial help (https://github.com/mifi/lossless-cut/issues/2921). Lesson: negative timestamps and jumps appear when concatenating without rewriting timestamps.

## 1b. Timeline model

### Comparison (real excerpts with link)

**OTIO** (https://opentimelineio.readthedocs.io/en/latest/tutorials/otio-timeline-structure.html, accessed 2026-11-14). **FACT**
- The timeline contains a top `Stack`, which contains `Track`s; a Track contains Clip, Gap, Transition and nested compositions. The Stack stacks "from bottom to top".
- "Each `Clip` has a source_range, which specifies a trimmed segment of media"; it can be None (whole media).
- "each media reference has a target_url and an available_range".
- "a `Gap` is meant to be transparent"; Transition "blends two adjacent items on the same track without changing the track's length" (that is, the transition is its **own item** that overlaps its neighbors via in/out offsets).
- The tutorial doc has no JSON; I did not paste the schema JSON.

**MLT XML (Shotcut/Kdenlive)** (https://www.mltframework.org/docs/mltxml/). **FACT**
```xml
<playlist id="playlist1">
  <blank length="50"/>
  <entry producer="producer1" in="0" out="149"/>
</playlist>
<tractor id="tractor0" in="0" out="199">
  <multitrack><track producer="playlist0"/><track producer="playlist1"/></multitrack>
  <transition id="transition0" in="50" out="74">
    <property name="a_track">0</property><property name="b_track">1</property>
    <property name="mlt_service">mix</property>
```
- The playlist is a sequential list per track with explicit `blank` for gaps; "All in/out points are absolute frame positions relative to the producer". The transition is a node in the tractor with a frame window and a reference to two tracks (a_track/b_track). Time is only in frames (resolution depends on the profile).

**Shotstack (declarative JSON)** (https://shotstack.io/docs/guide/llms-full.txt, I only read the first 100000 of 336691 characters). **FACT**
```json
{ "asset": {"type":"video","src":"...","trim":2,"volume":0.5},
  "start": 0, "length": 4,
  "transition": {"in":"fade","out":"fade"}, "filter":"boost", "effect":"slideRight" }
```
- Time in **float seconds**, clips positioned by `start`+`length` on tracks; "the first element is the top track" (array order = layer); clips on the same track cannot overlap; the transition is a **property of the clip** (`in`/`out`).

**Editly** (https://github.com/mifi/editly, accessed 2026-11-14). **FACT** for the model, **the JSON example was illustrated by the reading tool, not copied literally from the README**: clips in sequence, each clip has stacked `layers`; transition "at the end of this clip"; video `cutFrom`/`cutTo` is stretched to fit the clip duration. Without `--fast`, width/height/fps come from the first input video.

**Remotion**: the "model" is React (`Sequence`/`Series` with `durationInFrames` and `from`, time in whole frames per composition). **FACT** partial: it shows up in issues such as #11732 (`<Series>` with `trimBefore` on `<Audio>`) (https://github.com/remotion-dev/remotion/issues/11732). I did not open the `Sequence` doc.

**Kdenlive, Olive, FCPXML, CMX3600, Creatomate**: I did not open the schemas. See Gaps. From training [not verified]: CMX3600 is an event EDL with timecode and no layers; FCPXML uses rational time ("1001/30000s") and a storyline/spine with connected clips; Kdenlive serializes MLT.

### What this changes in our architecture (INFERENCE)

| Decision | Options seen | Recommendation for the core |
|---|---|---|
| Time | float seconds (Shotstack, Editly), frames (MLT, Remotion), rational (OTIO) | rational/whole frames; seconds only in the external API for agents, converted and validated at the edge |
| Position | clip with absolute `start` (Shotstack) vs sequence with gaps (OTIO/MLT) | **absolute position + `track`/layer** is easier for an agent to edit deterministically and for LWW per property; ripple/magnetic becomes a **command** (a macro of several sets), not a property of the model |
| In/out | `source_range` (OTIO), `in/out` (MLT), `trim` (Shotstack) | `source_in` + `duration` in timeline units, plus an explicit `speed` |
| Transition | own item on the track (OTIO, MLT tractor) vs clip property (Shotstack) | clip property with explicit `overlap` is simpler for per-property sync; own item is more expressive. OPINION: start with a clip property (in/out) |
| Text/captions | in Remotion/Editly they are layers | timeline item of type `text`/`caption` with `words[]` and times; see P11/P12 |
| Nesting | Stack/nested (OTIO), tractor (MLT) | postpone; only plan for `compound` in the schema (stable id) |
| Z-order | array order (Shotstack: first = top) vs bottom-up (OTIO) | choose one and document it; fractional index (see 1c) |

## 1c. Non-destructive editing, undo/redo and sync

### Comparison of patterns (INFERENCE, anchored in the sources)

| Pattern | Determinism | Serialization | Human + agent | Audit | Undo per actor |
|---|---|---|---|---|---|
| Command with inverse | good, if the inverse is computed in the application (stores the previous state) | commands are JSON | good, it is the agent's natural API | good (the log is the trail) | hard if there is interleaving: the inverse assumes the state it was applied to |
| Immutable snapshots | excellent | expensive if frequent (shared structures help) | good | weak (no intent) | weak; global undo |
| Patches (Immer/JSON Patch) | good, inverse patches for free | patches are already JSON | medium | medium | same limitation as the inverse |
| Event sourcing (ordered log of operations) | excellent: the log is reduced | the log is the format | excellent, actor in the event | excellent | undo becomes a **new compensating event** from the actor |
| CRDT (Yjs/Automerge) | guaranteed convergence, but the "intent" is diffuse | binary, less readable schema | good for offline/P2P | medium | Yjs: UndoManager by `origin` |

**Evidence**

- **FACT** Figma: authoritative server ("the server is the ultimate authority on what the document looks like"), one process per document, the server sets the order of events "so Figma doesn't need timestamps", **last-writer-wins per object property** ("the document will just end up with the last value that was sent to the server"), edits to different properties do not conflict, and it rejects updates that would break the document (parenting cycle). OT was ruled out: "OTs were unnecessarily complex for our problem space"; they did not implement a true CRDT because CRDTs serve decentralized systems. Children are ordered by **fractional index** (a fraction between 0 and 1). **Undo:** "the document should not change" if you undo a lot, copy and redo back to the present; undo and redo "adjust each other's history" and do not overwrite others' edits (https://www.figma.com/blog/how-figmas-multiplayer-technology-works/, accessed 2026-11-14).
- **FACT** tldraw sync: `TLSocketRoom` keeps the authoritative copy; a single room per document globally, otherwise "users won't see each other and will overwrite others' changes"; migrations and schema validation allow clients of different versions. The page I opened does not detail rebase or LWW (https://tldraw.dev/docs/sync, accessed 2026-11-14).
- **FACT (secondary source, unofficial reverse engineering)** Linear: `lastSyncId` increments with each transaction executed on the server, acting as the database version; clients fetch deltas if they are behind; client transactions stay in IndexedDB offline and are resent; the local database is only updated when the delta arrives from the server; the deltas may differ from the original transaction because of server side effects (search result citing https://github.com/wzhudev/reverse-linear-sync-engine, not read in full). Linear's official post "scaling the linear sync engine" appears in the result (https://linear.app/now/scaling-the-linear-sync-engine), but **I did not read it**; the one on "rebuilding delta sync read path" describes an "immutable ordered sync action log" per workspace (via the search summary, not opened).
- **FACT** Yjs `UndoManager`: has `trackedOrigins`; by default it tracks local changes without an origin, and a `captureTimeout` of 500 ms groups edits. The page does not describe per-user undo (https://docs.yjs.dev/api/undo-manager, accessed 2026-11-14).

### Conclusion (INFERENCE/OPINION)

- **An authoritative server with an ordered operation log + LWW per property is sufficient without CRDT** for our case (few actors, always with a central authority, discrete properties: position, duration, text, volume). This is exactly Figma's model, which treats a design doc (objects with properties) the same as our timeline doc (items with properties). The weak point is **caption text edited simultaneously**: Figma treats text as an atomic property; for short captions that is enough.
- **Sources that support the log**: every command from the agent and from the UI becomes a serializable `Operation` `{id, actor, baseRev, op, args}`; deterministic reduce `state' = apply(state, op)`. The log becomes the audit trail and the basis for "reproducible" runs.
- **Undo per actor** (INFERENCE): undo = **generate a compensating operation** from the current state **of the properties that actor touched**, discarded if another actor changed the same property afterwards (Figma's policy: "undo and redo adjust each other's history"). Undo of **side effects** (upload, exported file, transcoding) is not part of the model: treat them as **immutable artifacts addressed by hash** (see 1e/1f) and never "undo" the file; see P8.
- **Layer order**: fractional index (Figma) for z-order/track order avoids renumbering and causes fewer conflicts.

## 1d. Decode, composition, encode pipeline

**Sync and master clock.** [training] Standard pattern in players: audio as the master clock (the hardware consumes samples at a fixed rate) and video syncs by PTS. Evidence of a preview failure: Remotion #7300 (`<Video>` in `<Player>`: 25 fps sources in a 30 fps composition go out of sync and drift back ~4 frames; the author traces three seek branches with a fixed 0.15 s tolerance; the maintainer closed it for lack of a repro) (https://github.com/remotion-dev/remotion/issues/7300, accessed 2026-11-14) and motion-canvas #957 (MP3 audio misaligned after seek; fix: convert to WAV) (https://github.com/motion-canvas/motion-canvas/issues/957). **INFERENCE:** seek in MP3 is imprecise because of VBR/priming; the report itself fixes it by converting to a format with exact access.

**Priming, padding and edit lists (AAC).** **FACT** mediabunny #447 (https://github.com/Vanilagy/mediabunny/issues/447): an MP4 of 1 s reads 1.067 s because the AAC encoder adds priming that the file does not declare; "WebCodecs emits no information about any internal shifting or encoder priming delay"; the measured priming is **2112 samples (Chrome macOS and WebKit), 0 (Chrome Windows), 2048 (Chrome Android)**; the fix detects priming empirically per device and shifts timestamps; "For ISOBMFF, it works perfectly via the edit list"; in Matroska it uses `CodecDelay`; the end trim was left out. **FACT** w3c/webcodecs #626 (open): the API does not return priming/padding, so you cannot mux correctly or loop with sample accuracy (https://github.com/w3c/webcodecs/issues/626). **FACT** Remotion #7099 and #12703: final padding that inflates duration still remains, and +46.4 ms in HLS audio (`elst` media time 2048 at 44.1 kHz) (https://github.com/remotion-dev/remotion/issues/12703). **Safari**: the AAC AudioEncoder treats the first 1024 samples as silence (https://bugs.webkit.org/show_bug.cgi?id=328491, listed at https://www.remotion.dev/docs/mediabunny/webcodecs-bugs).

**B-frames and PTS.** **FACT** mediabunny #644 (open as of 2026-11-12): when reordering, PTS `0,3,1,2,6,4,5`; the author replies "VideoDecoder is not allowed to omit outputs, by spec" (https://github.com/Vanilagy/mediabunny/issues/644). Chromium: "Software AVC VideoDecoder drops initial B-frames" (fixed in 156) and w3c/webcodecs #519 "missing three frames when decoding h264 video" (https://github.com/w3c/webcodecs/issues/519). **Lesson**: the exporter must never assume PTS = frame index; use the PTS returned by the decoder.

**Precise seek and long GOP.** [training] Exact seek requires decoding from the previous keyframe up to the target frame (cost = distance to the keyframe). Evidence of the pain: LosslessCut #3210 (a segment in keyframe mode starts at the next keyframe, MPEG-TS) (https://github.com/mifi/lossless-cut/issues/3210) and #126 "smart cut", open for a long time (https://github.com/mifi/lossless-cut/issues/126). Chromium: "VideoDecoder rejects non-IDR AVC key frames" (open) and "HEVC VideoDecoder key frame detection too strict" (fixed 149) (Remotion's list). **INFERENCE**: all-intra proxies (or short GOP) are what make scrubbing smooth; see 1e.

**VFR.** **FACT** OBS #13396: NVENC/AMF/QSV recording at 30 fps has VFR in the timestamps (29.412 to 30.303 fps), "even when CBR" (CBR controls bitrate, not frame interval); the maintainer closed it as a duplicate and an "FFmpeg issue" (https://github.com/obsproject/obs-studio/issues/13396). **FACT** Shotcut: when importing an iPhone `.mov`, the app asks whether to convert because it is VFR (step 4 of the repro at https://github.com/mltframework/shotcut/issues/1791; the case itself was the Linux audio device, not the VFR). **FACT** mediabunny #320 (open): asks for a "field for variable frame rate" in PacketStats (https://github.com/Vanilagy/mediabunny/issues/320). **INFERENCE/OPINION**: **normalizing to CFR at ingest for phone videos and screen recordings** (proxy or normalized copy) is the decision that most avoids frame-selection bugs; the original stays untouched for the final export only if the export also resolves VFR per frame, otherwise export from the normalized copy.

**Rotation/display matrix.** **FACT** Remotion #5509: an iPhone with an orientation tag came out with the wrong rotation on conversion (cause: the `convertMedia` options of its own) (https://github.com/remotion-dev/remotion/issues/5509). **FACT** mediabunny #492: files can have scale, translation and mirroring in the `mvhd` and `tkhd` matrices, not just rotation; fixed with `getTransformationMatrix()` and `getFlip()`; "reading any video that contains any combination of rotation and flip works correctly... That's all that Chrome supports anyway" (https://github.com/Vanilagy/mediabunny/issues/492). Also mediabunny #403 and #337 (conversion ignores rotation when resizing). **Lesson**: apply the combined matrix once at ingest (or in the sampler) and have a test with rotated/mirrored videos.

**HDR/10-bit/HLG (iPhone).** **FACT** Shotcut forum: an iPhone HLG clip (`Color Transfer ARIB B67 (HLG)`) converted with "Convert to BT.709" came out washed out; the developers: "converting HDR to SDR is inherently lossy, and there is more than one way to do it"; they suggest the NBCU HLG2SDR LUT (https://forum.shotcut.org/t/iphone-hdr-washed-out/47286, accessed 2026-11-14). **FACT** Premiere: a report of iPhone HLG washed out in Premiere 26.3 Windows (https://community.adobe.com/bug-reports-728/iphone-rec-2100-hlg-footage-becomes-washed-out-in-premiere-pro-on-windows-1635208, search summary only). **FACT** Chromium (Remotion's list): "VideoDecoder ignores colorSpace for H.264 on macOS VideoToolbox, outputs bt709" (https://issues.chromium.org/issues/532860177), "HDR HEVC VideoFrame draws onto canvas as fully transparent (regression), fixed in 148" (https://issues.chromium.org/issues/504680933), "VideoFrame format I444AP12 not recognized". **FACT** w3c/webcodecs #940: the colorSpace override is not well defined, and Chrome and Firefox diverge for an isolated `transfer: 'PQ'`; Safari/VideoToolbox declares a fully specified colorSpace that the UA is assumed to give precedence (https://github.com/w3c/webcodecs/issues/940). **INFERENCE**: we need an **explicit color policy**: detect the source's `transfer`/`primaries`/`matrix`/`range`, and for HLG/PQ apply a documented tone mapping to limited SDR BT.709 (with controlled desaturation) already at ingest, and test with real iPhone videos. [training]: the typical path in FFmpeg is `zscale` + `tonemap`; I did not verify the parameters today.

**BT.601/709 and range (washed-out color).** I did not find a specific open and read issue about full vs limited. **FACT** adjacent: Shotcut #1517 "Text overlay shifts levels slightly" (https://github.com/mltframework/shotcut/issues/1517, I only opened the title). Treat it as a gap and as a **mandatory regression test**: a clip with color bars + ramp in both ranges, comparing preview (canvas) and export frame by frame.

**Alpha.** No source read; gap.

**Audio: resampling, mixing, loudness, ducking.**
- **FACT (weak)** Loudness targets: searches returned only third-party sources (blogs/tools), no official YouTube, TikTok or Instagram page; the recurring number is **-14 LUFS integrated, true peak -1 dBTP**; one result says TikTok does not publish an official target. Treat as **not verified** (search on 2026-11-14, no official link opened).
- **INFERENCE**: mix in 32-bit float at 48 kHz, measure LUFS on the offline render (2 passes) and **not** on the preview; ducking as keyframe automation generated by a command (so it stays auditable and reversible).

## 1e. Proxies, thumbnails, waveforms, cache

- **FACT** Browser memory data: MDN says "applications can crash with fewer than 100 active frames in memory" according to a search summary (https://developer.mozilla.org/en-US/docs/Web/API/WebCodecs_API/Using_the_WebCodecs_API, not opened in full). The spec says codec resources "MAY be quickly exhausted and SHOULD be released immediately" and that authors should call `close()` immediately (https://w3c.github.io/webcodecs/, excerpt opened). Firefox logs a warning for a frame collected without `close()` (https://bugzilla.mozilla.org/show_bug.cgi?id=1887554, via search summary).
- **FACT** mediabunny #144: a leak of ~3.3 MB per frame in `VideoSampleSink.samples()` on ~500 GB uploads; the cause was file-read memory that the browser never collected; worked around by cancelling the reader and reported to Chromium (https://issues.chromium.org/issues/554940580); v1.55.5 stabilized the RAM (https://github.com/Vanilagy/mediabunny/issues/144).
- **FACT** diffusionstudio/core #63: a composition with 200 `subclip(0,1)` of a 78 MB video freezes "at one certain number based on the file size"; the response: v2 has "major improvements in memory management" (https://github.com/diffusionstudio/core/issues/63).
- **INFERENCE**: cache architecture = (1) **ingest proxy** all-intra/short GOP, CFR, orientation and color already normalized, addressed by content hash + proxy profile version; (2) decoded frame cache **LRU by bytes** (not by count), always with deterministic `close()`; (3) thumbnails and waveform peaks computed once at ingest and persisted (peaks at multiple zoom levels) under the same asset hash key; (4) invalidation = changing the key (file hash + pipeline version), never mutating the artifact.

## 1f. Project format and migrations

- **FACT** Olive #1700: "Olive refuses to open a project file if too old"; the maintainer: format-breaking changes are unavoidable and "generally there hasn't been any effort in 0.2 to 'upgrade' projects when the format is bumped" (https://github.com/olive-editor/olive/issues/1700). Olive #2229: a 0.2 project does not open in the nightly ("Failed to determine project version"); the format is now compressed XML by default and "I wouldn't be confident that it would be easier to upgrade the file than it would be to remake it" (https://github.com/olive-editor/olive/issues/2229).
- **FACT** tldraw sync: "Migrations and schema validation are meant to let clients of different versions... collaborate" (https://tldraw.dev/docs/sync): migrations as part of the sync contract.
- **INFERENCE (recommendation)**: an integer `schemaVersion` at the top; pure chained migrations `vN -> vN+1` with fixtures of old projects in the tests (one per version, forever); detect the version before any detailed parsing; **never** lose unknown fields (preserve them in `extras`) on save; media referenced by `{id, contentHash, relative path, original name, probe metadata}` for relinking by hash; an exportable bundle (zip with project + media + fonts). Interchange formats (OTIO) only for import/export, not as the native format.
- **Offline media**: no issue read; gap (see Gaps).

---

# PART 2: POST-MORTEMS

Format per case: what happened, root cause, resolution, **LESSON**.

## P1. Preview ≠ export (text, fonts, colors, position, timing)

1. **Remotion #12603, preview 1 frame behind the render (WebM).** Preview and render choose frames with different rounding rules (`roundTo4Digits` and a 1 ms tolerance in the render). Closed on 2026-10-25. https://github.com/remotion-dev/remotion/issues/12603. **LESSON:** frame selection by time is a single pure function, used by preview and export, with edge tests in ms.
2. **Remotion #11732, `<Audio>`: during the first 5 to 60 ms of each clip in sequence the audio disappears in the preview; the render is correct.** Closed 2026-09-27. https://github.com/remotion-dev/remotion/issues/11732. **LESSON:** audio preview and audio render are distinct paths; the preview needs pre-scheduling (premount) and the **offline render is the truth**; hard cuts (jump-cut) are the test case.
3. **Remotion #11315, text inside `<svg>` renders with a fallback font in the web renderer.** Cause: the SVG is serialized and loaded as an `<img>`, an isolated document without the page's fonts. Closed, fixed (including for `@remotion/google-fonts`). https://github.com/remotion-dev/remotion/issues/11315. **LESSON:** any "serialize DOM to bitmap" path loses loaded fonts; embed the fonts (data URL) or draw the text yourself on canvas.
4. **Remotion #5984, different font in the Docker (Linux) render versus Studio, in Lottie text (`tspan`).** A comment: "when running the render locally (Mac) I did not have these problems! It must be something with the docker image (Linux)". https://github.com/remotion-dev/remotion/issues/5984. **LESSON:** system fonts vary per machine; the render may only use fonts bundled with the project.
5. **Remotion #12604, text baseline shifted ~0.45 px inside an ancestor with `scale(0.9)` when precompositing rasterizes at 1x.** `fillText` snaps the baseline to a whole pixel. https://github.com/remotion-dev/remotion/issues/12604. Also #12593 (`mix-blend-mode` ignored in `renderMediaOnWeb`) and #13054 (text-shadow repaints the glyph; text under opacity ends up too opaque). **LESSON:** a browser renderer for export partially reimplements CSS; the **set of supported effects must be small and defined by us** (not "any CSS").
6. **OpenShot #5747, audio sped up and distorted in the preview, correct in the export.** https://github.com/OpenShot/openshot-qt/issues/5747 (closed with "try the RC"; cause not recorded). **LESSON:** audio preview needs its own test suite.
7. **Olive #1705, after deleting/moving an audio clip and undoing, the old sound still plays in the preview** (and persists after saving and reopening). https://github.com/olive-editor/olive/issues/1705. **LESSON:** preview cache must invalidate by model revision; the render graph derives from the state, it is not maintained by hand.

## P2. Audio drift and desync

1. **mediabunny #447 / Remotion #7099 / #12703**: AAC priming and padding, edit list, 46.4 ms in HLS. See 1d. Root cause: WebCodecs does not report priming; the measurement varies by platform. **LESSON:** mediabunny already compensates for priming at the start; trimming the end is still pending. Our export must **validate the final file duration** (probe) against the timeline and fail/warn.
2. **MLT #69**: audio 1 frame late with an in-point in compressed formats. https://github.com/mltframework/mlt/issues/69. Discard `(our_time - stream_time) * rate` samples. **LESSON:** cutting an audio source is done in samples, not in frames.
3. **Editly #117 and #327**: audio goes out of sync when the source audio tracks have different lengths; workaround with `detached-audio`; the concatenated audio is shorter with `keepSourceAudio` (https://github.com/mifi/editly/issues/327; #117 listed by search at https://github.com/mifi/editly/issues/117, not opened). Cause: concatenating audio per clip without filling silence up to the exact clip duration. **LESSON:** each audio clip is rendered with exact duration (fill silence) in samples.
4. **motion-canvas #957**: after seek, MP3 misaligned; WAV fixes it. https://github.com/motion-canvas/motion-canvas/issues/957.
5. **Remotion #7300**: see 1d (preview, 0.15 s tolerance).

## P3. Memory and crashes in the browser

1. **mediabunny #144**: leak of ~3.3 MB/frame (see 1e). **LESSON:** rehearse large uploads; test RAM stability in a loop.
2. **ffmpeg.wasm #876 (4 GB)**: files over 2 GB are impossible; a 4 GB request open since 2025-06-16; response: "ffmpeg.wasm team is pretty bandwidth limited... happy to look at patches" (https://github.com/ffmpegwasm/ffmpeg.wasm/issues/876). **#451**: reducing initial memory in 0.11 caused OOM vs 0.10 (https://github.com/ffmpegwasm/ffmpeg.wasm/issues/451). **LESSON:** ffmpeg.wasm has a memory ceiling (32-bit WASM) and maintainers with limited bandwidth; it must not be the critical path for export. The use of SharedArrayBuffer/COOP-COEP also generates a series of issues (#263, #484, #245).
3. **OpenCut #914**: a 20 min video export "stuck at 5%", no error; with 40 min in Chrome it reaches 100% and fails with "Array buffer allocation failed" (https://github.com/OpenCut-app/OpenCut/issues/914, 2026-10-18). **LESSON:** single-memory export (ArrayBuffer) does not scale; use a streaming muxer to a file (File System Access/OPFS) and show the real error.
4. **diffusionstudio/core #63**: see 1e. **Editly #72** (memory leak on a long video, 200 clips; only the title read).
5. **Chromium**: "VideoEncoder slows drastically when the page re-renders" (https://issues.chromium.org/issues/414491381, open) and "High-refresh (144+ Hz) monitor throttles VideoDecoder 5-10x" (https://issues.chromium.org/issues/552063578, open). **LESSON:** export in a Worker/offscreen, without depending on the UI.
6. **MDN/Firefox**: crash on decoder `close()` in Firefox 140.0.4 according to a comment in the Rerun code (re_video) (via search result, not opened).

## P4. VFR and phone/screen-recording videos

1. **OBS #13396** (VFR from a hardware encoder; see 1d).
2. **Shotcut** asks to convert VFR (iPhone .mov) (see 1d).
3. **LosslessCut #2338/#455** (different timebases and framerates on merge) and #2921 (timestamp jumps).
4. **Remotion #10274**: `<OffthreadVideo>` renders wrong frames (~3 frames early) under default concurrency, **non-deterministic**, and disappears with `--concurrency=1`; "silently incorrect": no error (https://github.com/remotion-dev/remotion/issues/10274). Closed: the author can no longer reproduce it. **LESSON:** the export needs a check that repeated renders give the same result (see P16) and a concurrency-1 mode for debugging.
5. **HandBrake/OBS**: HandBrake has VFR/CFR as an option [training]; not opened.

## P5. Unsupported codecs and containers

1. **mediabunny #592**: HEVC asks for a third-party decoder extension doc (open) (https://github.com/Vanilagy/mediabunny/issues/592). **#365**: AMD hardware HEVC on Windows/Chrome emits the first chunk as `delta` and the muxer fails ("First packet must be a key packet"); fixed in v1.45.3; in Chromium marked as won't fix (https://issues.chromium.org/issues/514397892) (https://github.com/Vanilagy/mediabunny/issues/365).
2. **Chromium** (Remotion's list): interlaced content on a hardware decoder fails (fixed 144), `AudioDecoder.isConfigSupported()` with false positives (fixed 130), audio with negative timestamps ignored (fixed 156), VideoEncoder VP8/VP9 fails with a zero-duration frame (fixed 142). https://www.remotion.dev/docs/mediabunny/webcodecs-bugs.
3. **Safari/WebKit**: "VideoDecoder outputs frames in the wrong order" (fixed, bug 287516), "WebM audio plays back garbled when BlockGroup is used" (fixed, bug 297781).
4. **Remotion #11586**: odd-dimension compositions do not export to H.264/H.265 (https://github.com/remotion-dev/remotion/issues/11586, only the title read).
5. **OpenCut #912/#915 and #832**: "Sound export issue", "EXPORT ERROR" (open; only titles read).
**LESSON:** `VideoDecoder.isConfigSupported` and `VideoEncoder.isConfigSupported` are only one step; we need **real per-device capability probing at ingest** (decode 1 frame) and a **transcoding fallback** on desktop (FFmpeg). Assume H.264 + AAC as the only **output** format; accept HEVC/MOV/ProRes on input via the desktop path. I did not verify AV1/ProRes/AC3/PCM against sources; I only saw the AV1 item mediabunny #423 (sync samples without Sequence Header OBU) in its title.

## P6. Rotation

1. **Remotion #5509** and **mediabunny #492, #403, #337** (see 1d). **LESSON:** normalize the display matrix at ingest and test with portrait and mirrored iPhone videos.

## P7. Licenses

1. **Remotion (source-available license with commercial terms)**: #9653 "Remotion Licensing masterplan" (open, 2026-07-25) lists goals: keep the business sustainable, "enforce licensing - terms, telemetry, and license itself"; many linked items are private (https://github.com/remotion-dev/remotion/issues/9653). **#13403** (2026-11-14): a user asks for clarification because `@remotion/effects` 4.0.542 declares `UNLICENSED` and `@remotion/media` and the compositor omit a declaration (https://github.com/remotion-dev/remotion/issues/13403). **#12605** (incomplete telemetry in the render APIs). I could not open the official pricing/license page (the route `/docs/license-pricing` returned 404 and `/docs/license` is only an index). **LESSON:** **declare the license in each package** and keep the policy simple; a dependency with commercial terms or mandatory telemetry is a risk for an MIT project. Do not use Remotion as a core dependency (OPINION).
2. **FFmpeg**: LGPL 2.1+ license with optional GPL parts; "If those parts get used the GPL applies to all of FFmpeg"; for LGPL compliance: no `--enable-gpl`/`--enable-nonfree`, dynamic linking, provide the corresponding source, attribution, and "not using any GPL libraries (notably libx264)" (https://ffmpeg.org/legal.html, accessed 2026-11-14). **LESSON:** since the project is MIT and distributes a desktop app, an FFmpeg binary bundled with libx264 is GPL; use hardware/Media Foundation/VideoToolbox/LGPL encoders and meet the checklist, or **do not distribute the binary** (ask the user). I did not find real license-violation cases with an open source (the page does not list them; see Gaps). Shotcut/Kdenlive/Olive are GPL and may not be reused in MIT code [training].
3. **Fonts**: no case read; gap.

## P8. Undo/redo

1. **Olive #1705** (undo does not restore the preview/audio state; it persisted after saving).
2. **Olive #2132** (crash with Ctrl+Z after Glide-Tool), **#1523** (undo of an audio node crashes), **#1781** (moving 2 clips with cross-dissolve crashes): only the titles were read; they indicate undo + transition as a fragile area (https://github.com/olive-editor/olive/issues/2132, /1523, /1781). INFERENCE: transitions reference two items, and the inverse must rebuild the reference.
3. **Figma**: undo principle in multiplayer (see 1c).
4. **Yjs**: `captureTimeout` groups edits; command boundaries must be defined (`stopCapturing`).
**LESSON:** (a) every command needs an "apply, undo, redo = identical state" test per property (property-based); (b) **side effects** (upload, transcoding, export) stay **outside** the history, represented as immutable artifacts; the undo command only undoes the **reference**.

## P9. Project format (compatibility break, offline media)

1. **Olive #1700 and #2229** (see 1f). 2. **OpenShot #5869** "crashes when I attempt to open specific project file" and #5901 "opening new project overwrites existing project edits" (titles read: https://github.com/OpenShot/openshot-qt/issues/5869, /5901). **LESSON:** atomic saving (write to a temp file and rename), validate on open, and keep an automatic backup of the previous version before migrating. Offline media: gap.

## P10. Fonts and text rendering (fallback, emoji, RTL/CJK, shaping)

1. **Remotion #11315, #5984** (see P1).
2. **Editly #161**: "Urdu not centered in text component" (open; title only) (https://github.com/mifi/editly/issues/161). Editly #135 "Could not parse font file".
3. **libass/FFmpeg**: search result from the Render community: "libass is still using system fontconfig instead of my bundled fonts even when passing fontsdir to the subtitles filter" with no answer (https://community.render.com/t/ffmpeg-libass-subtitles-filter-not-using-fontsdir-parameter-on-render-deployment/39185, search result, not opened). Emby doc: a fallback font can change width, baseline and line breaks (https://emby.media/learn/articles/subtitles/rendering/fonts-typesetting/Subtitle-Font-Fallback.html, via search). FFmpeg trac ticket #9002 (the `subtitles` filter does not display all ASS captions) (https://fftrac-bg.ffmpeg.org/ticket/9002, via search).
**LESSON / INFERENCE:** **a single text layout implementation** serves preview and export (canvas/WebGPU with bundled fonts, `Intl.Segmenter` and HarfBuzz via WASM if we want identical shaping in browser and desktop). If export uses libass/drawtext, there will be two implementations and divergence in line breaks, kerning and emoji. OPINION: render captions as our own frames/layers and **not** use `subtitles=`/`drawtext` for burn-in.

## P11. Captions (line breaks, safe area, karaoke, burn-in)

- **FACT** In ASS, `\k` is in centiseconds (100 = 1 s); `\k` switches in steps, while `\K`/`\kf` sweep (https://emby.media/learn/articles/subtitles/rendering/fonts-typesetting/Subtitle-Karaoke.html, via search, not opened). WebVTT uses another model (past/future cue).
- **FACT** Remotion #12927 "Resource suggestion: caption retiming across cuts and speed changes" (open) (https://github.com/remotion-dev/remotion/issues/12927; title read only). **INFERENCE:** word-level captions with times on the **source timebase**, derived for the timeline by mapping, to survive cuts and speed changes.
- Platform 9:16 safe area: no source read; gap.

## P12. Collaboration/sync

- Figma, tldraw, Linear (see 1c). **LESSON:** the server + log + LWW model works for object editors; the real risk is **order and idempotency** (operation id, `baseRev`), not fine-grained conflict resolution.

## P13. Slow or stuck export and long jobs

1. **OpenCut #914** (stuck at 5% with no error, 20 to 40 min). 2. **Shotcut #1774** "mp4 Export Video freezes at 36:25:440" (closed; title read: https://github.com/mltframework/shotcut/issues/1774). 3. **OpenShot #5392** "Freezes when Exporting MP4" (title). 4. **Chromium** VideoEncoder slow when the page re-renders (open). **LESSON:** export as a **job in a worker** with real per-frame progress, cooperative cancellation, typed errors, and resumption by **segments** (independent chunks with a closed GOP, concatenated at the end). Resumption: no source; INFERENCE.

## P14. Large file uploads

- **FACT** mediabunny #144 (500 GB in batch), ffmpeg.wasm #876 (2 GB). **LESSON:** local-first: never load the whole file into memory; read via `File.slice`/stream; for the server, multipart/resumable upload (no source read; INFERENCE).

## P15. Cross-browser compatibility

- **Firefox**: AVC VideoEncoder produces invalid AVCC (https://bugzilla.mozilla.org/show_bug.cgi?id=2082867) and shifts timestamps by the duration (bug 2082890). **Safari**: AAC AudioEncoder with 1024 silence samples. The target is Chrome; even so, the desktop path avoids these bugs.

## P16. Render determinism

1. **Remotion #10274** (identical renders produce artifacts on different frames; `--concurrency=1` reproduces identical bytes). 2. **Remotion #11578** (sustained renders on Linux capture the previous frame's state: 56 of 75 frames identical and stale; resolved as "non-bug" after the author acknowledged an error) (https://github.com/remotion-dev/remotion/issues/11578). 3. **mediabunny #365** (AMD hardware HEVC encoder diverges in the type of the first chunk). 4. **Hardware encoders**: [training] NVENC/QuickSync/VideoToolbox are not bit-exact across machines; I have no open source today. **LESSON:** define determinism at **two levels**: (a) the **sequence of composed frames** (pixels before encoding) must be identical across machines, testable by hash; (b) the final file depends on the encoder and is **not** promised to be bit-exact (hash only with the software encoder and a fixed version).

---

# The 12 lessons that most change the architecture

1. **Rational/integer time in the core; float only at the edge.** Evidence: Remotion #12603 (different rounding between preview and render), Editly #49 (duration varies with fps), mediabunny #69 ("be really diligent... convert to some integer coordinate system"), flicks do not cover NTSC. Decision: `{num,den}` or frames at the project rate (30000/1001) + 48 kHz samples at the edge of the mixer.
2. **A single function "frame/sample for time t" and a single renderer for preview and export.** Evidence: Remotion #12603, #11315, #12604, #5984, #11732, OpenShot #5747. Preview may be degraded (resolution), but it must never diverge in semantics.
3. **Authoritative server with an ordered operation log and LWW per property, without CRDT.** Evidence: Figma (blog), tldraw (authoritative room), Linear (lastSyncId, secondary source). Operations with `id`, `actor`, `baseRev`, deterministic reduce.
4. **Undo = compensating operation per actor; side effects outside the history, as immutable artifacts by hash.** Evidence: Figma (undo/redo adjust the history), Olive #1705, #2132, #1523, #1781.
5. **Normalize at ingest: CFR, orientation (combined matrix), color (BT.709 limited, HLG/PQ tone mapping), with an all-intra proxy.** Evidence: OBS #13396, Shotcut (asks to convert VFR), Remotion #5509, mediabunny #492, Shotcut forum HLG, Chromium #532860177, w3c #940.
6. **Explicit color policy and color-bar tests between canvas and export.** Evidence: washed-out HLG (Shotcut forum, Adobe), "converting HDR to SDR is inherently lossy... more than one way". Mandatory regression test.
7. **Audio in samples, with priming/padding handled at export and duration validated by probe of the final file.** Evidence: mediabunny #447 (2112/0/2048 per platform), w3c #626, Remotion #7099 and #12703, MLT #69, Editly #327.
8. **Explicit WebCodecs resource management: deterministic `close()`, LRU cache by bytes, export in a Worker, streaming to disk.** Evidence: WebCodecs spec, mediabunny #144, diffusionstudio #63, OpenCut #914 ("Array buffer allocation failed"), Chromium (slow encoder with re-render).
9. **Do not depend on ffmpeg.wasm for the critical path; FFmpeg only on desktop and with carefully handled LGPL licensing.** Evidence: ffmpeg.wasm #876 and #451 (2 GB ceiling/OOM, maintainers with limited bandwidth), FFmpeg legal (GPL if libx264), cost for an MIT project.
10. **Real capability probing (decode 1 frame) and transcoding fallback; trusting `isConfigSupported` is not enough.** Evidence: Chromium #360083330 (false positives), mediabunny #365 (AMD HEVC), #644 and the decoder that reorders/omits, Safari (frames out of order).
11. **A single text layout with bundled fonts; word captions on the source timebase; do not use libass/drawtext for burn-in.** Evidence: Remotion #11315 and #5984 (fallback font), libass `fontsdir` ignored, Emby (fallback changes width/line), Remotion #12927 (caption retiming).
12. **Project format with `schemaVersion`, chained migrations, fixtures per version, atomic saving and relink by hash; export with resumable jobs by segments and real progress.** Evidence: Olive #1700 and #2229 (old projects do not open; migration harder than redoing), tldraw (migrations as contract), OpenCut #914 (stuck at 5%, no error), OpenShot #5869/#5901.

---

# Gaps (what I could not or did not verify)

- **Schemas not opened**: Kdenlive, Olive (node model), FCPXML, CMX3600, Creatomate, Remotion `Sequence`. Only OTIO, MLT, Shotstack and (partially) Editly. The Editly JSON example above was "illustrated" by the extractor, not copied from the README. From Shotstack I only read 100000 of 336691 characters.
- **OTIO `RationalTime`**: I did not find the project rationale (why rational) in the docs; I only read the header.
- **Not covered**: bugs.kde.org/invent.kde.org (Kdenlive), mltframework/shotcut only via title search, engineering posts from Clipchamp/Microsoft, CapCut/ByteDance, Canva, Descript, Kapwing, Veed, Demuxed/Chrome Dev Summit talks. The search "webcodecs video editor engineering blog" did not find a team blog of an editor.
- **No open source**: real license-violation cases (GPL in binary, fonts), Remotion relicensing (only 2026 issues about license declaration); official platform LUFS target (only blogs); BT.601 vs 709 and full vs limited with a specific issue; alpha; AV1/ProRes/AC3/PCM; platform safe area; offline media/relink; export resumption; determinism across hardware encoders (only [training]).
- **Some issues had only the title read** (marked in the text as "title only"): Olive #2132/#1523/#1781, OpenShot #5869/#5901/#5392, Shotcut #1774/#1517, Editly #72/#117/#161, Remotion #11586/#12927, OpenCut #832/#912/#915.
- **Linear**: the technical basis is unofficial reverse engineering; the official posts appeared in the search but were not read.
- Everything marked [training] needs checking before it becomes a decision.
