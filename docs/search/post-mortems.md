# Post-mortems: where other editors got hurt

> Stage 1: Research. All sources accessed 2026-11-14.
> Markers: **FACT** (verified in the source), **INFERENCE**, **OPINION**.
> Full evidence: [`notes/engine-postmortems.md`](notes/engine-postmortems.md) (Part 2) and the profiles in [`projects/`](projects/). A sample of 13 issues cited here was re-checked by the architect (existence, title and state): [`notes/architect-verifications.md`](notes/architect-verifications.md), line 12.

Each topic ends with the rule it imposes on our specifications. That rule, not the case itself, is what the SDDs and the ROADMAP must carry.

## Index

1. [Preview differs from export](#1-preview-differs-from-export)
2. [Audio desync](#2-audio-desync)
3. [Memory and freezes in the browser](#3-memory-and-freezes-in-the-browser)
4. [Phone video: VFR, rotation and HDR](#4-phone-video-vfr-rotation-and-hdr)
5. [Codecs and containers](#5-codecs-and-containers)
6. [Text, fonts and captions](#6-text-fonts-and-captions)
7. [Undo and redo](#7-undo-and-redo)
8. [Project format](#8-project-format)
9. [Long, stuck or silently wrong export](#9-long-stuck-or-silently-wrong-export)
10. [Determinism](#10-determinism)
11. [Licensing](#11-licensing)
12. [Architecture stuck to the UI](#12-architecture-stuck-to-the-ui)
13. [Gaps](#13-gaps)

## 1. Preview differs from export

| Case | What happened | Root cause | Source |
| --- | --- | --- | --- |
| Remotion #12603 | Preview one frame behind the WebM render | Two rounding rules for choosing the frame | <https://github.com/remotion-dev/remotion/issues/12603> |
| Remotion #11315 | Text inside SVG renders with a fallback font in the web renderer | The SVG is serialized and loaded as an image, an isolated document without the page's fonts | <https://github.com/remotion-dev/remotion/issues/11315> |
| Remotion #5984 | Different font in the Docker (Linux) render and in the Studio (Mac) | System fonts vary per machine | <https://github.com/remotion-dev/remotion/issues/5984> |
| Remotion #12604, #12593, #13054 | Text baseline shifted, `mix-blend-mode` ignored, wrong text shadow under opacity | The export renderer partially reimplements CSS | <https://github.com/remotion-dev/remotion/issues/12604> |
| FreeCut #92, #108, #137, #250, #349 | Series of divergences between preview and export | Playback uses DOM/React composition; scrub and export use canvas/WebGPU: two pixel paths | [`projects/freecut.md`](projects/freecut.md) |
| Reframe #1498 | Preview and export diverge | Preview in DOM/CSS, export in an FFmpeg filtergraph | [`projects/reframe.md`](projects/reframe.md) |
| OpenShot #5747 | Audio sped up and distorted in the preview, correct in the export | Not recorded | <https://github.com/OpenShot/openshot-qt/issues/5747> |

**Rule.** (1) "Which source frame belongs to time t" and "where each layer is at time t" are single pure functions, shared by preview and export. (2) The set of supported visual effects is small and defined by us, never "any CSS". (3) The render uses only fonts packaged with the project, never system fonts. (4) Where there are two pixel executors, the accepted difference between them is a defined number, verified by reference-frame tests.

## 2. Audio desync

| Case | What happened | Root cause | Source |
| --- | --- | --- | --- |
| mediabunny #447 | A 1 s MP4 reads as 1.067 s | AAC priming not declared; varies by platform (2112, 0 or 2048 samples) | <https://github.com/Vanilagy/mediabunny/issues/447> |
| Remotion #7099, #12703 | Inflated duration; 46.4 ms drift in HLS | Trailing AAC padding; edit list | <https://github.com/remotion-dev/remotion/issues/12703> |
| MLT #69 | Audio one frame late with an in-point | Audio cut computed in frames, not samples | <https://github.com/mltframework/mlt/issues/69> |
| Editly #327 | Concatenated audio shorter than the video | Per-clip audio without fill silence | <https://github.com/mifi/editly/issues/327> |
| motion-canvas #957 | MP3 misaligned after seek; WAV resolves it | Imprecise seek in MP3 | <https://github.com/motion-canvas/motion-canvas/issues/957> |
| OpenCut #812, #915, #628 | Audio missing or wrong in the export | Preview uses streaming WebAudio; export mixes offline through another path | [`projects/opencut.md`](projects/opencut.md) |

**Rule.** Audio is cut and mixed in samples. Each item yields exactly its duration, padded with fill silence. Ingest converts audio to an exact-access format. The export validates the final file's duration against the timeline and fails explicitly if they diverge.

## 3. Memory and freezes in the browser

| Case | What happened | Source |
| --- | --- | --- |
| mediabunny #144 | Leak of about 3.3 MB per frame when reading samples | <https://github.com/Vanilagy/mediabunny/issues/144> |
| OpenCut #914 | 20 min export stuck at 5% with no error; at 40 min, "Array buffer allocation failed". The file is assembled entirely in memory | <https://github.com/OpenCut-app/OpenCut/issues/914> |
| FreeCut #316 | In-memory mux fails on a long timeline | [`projects/freecut.md`](projects/freecut.md) |
| ffmpeg.wasm #876, #451 | Files above 2 GB are impossible; a 4 GB request open since 2025; maintainers say "pretty bandwidth limited" | <https://github.com/ffmpegwasm/ffmpeg.wasm/issues/876> |
| diffusionstudio/core #63 | Composition with 200 subclips freezes at a point that depends on file size | <https://github.com/diffusionstudio/core/issues/63> |
| Chromium 414491381, 552063578 | `VideoEncoder` becomes slow when the page re-renders; a 144 Hz monitor reduces `VideoDecoder` by 5 to 10 times (open) | <https://issues.chromium.org/issues/414491381> |
| voidcut | Media stored as data URLs | [`projects/voidcut.md`](projects/voidcut.md) |

**Rule.** Exporting a video of up to 5 min never depends on fitting in a tab's memory. No ffmpeg.wasm on the critical path. In the browser: deterministic `close()` on every `VideoFrame`, cache bounded by bytes, heavy work off the UI thread, file reads in chunks.

## 4. Phone video: VFR, rotation and HDR

| Case | What happened | Source |
| --- | --- | --- |
| OBS #13396 | A "30 fps" recording from a hardware encoder has real intervals between 29.4 and 30.3 fps | <https://github.com/obsproject/obs-studio/issues/13396> |
| LosslessCut #455, #2338, #2921 | Sources with different fps and timebases freeze or fail when joined; timestamp jumps | <https://github.com/mifi/lossless-cut/issues/2921> |
| Remotion #5509 | iPhone video with wrong rotation after conversion | <https://github.com/remotion-dev/remotion/issues/5509> |
| mediabunny #492 | Files carry scale, translation and mirroring in the display matrix, not just rotation | <https://github.com/Vanilagy/mediabunny/issues/492> |
| Shotcut forum | iPhone HLG looks "washed out" when converted to BT.709; "converting HDR to SDR is inherently lossy, and there is more than one way to do it" | <https://forum.shotcut.org/t/iphone-hdr-washed-out/47286> |
| Chromium 532860177 | `VideoDecoder` ignores the H.264 `colorSpace` on macOS and returns BT.709 | <https://issues.chromium.org/issues/532860177> |

**Rule.** Ingest produces normalized media (constant frame rate, display matrix applied, SDR BT.709), and preview and export both read from it. The ingest test suite must include: iPhone portrait video, mirrored front-camera video, HLG video, VFR screen recording.

## 5. Codecs and containers

- **FACT.** AMD hardware HEVC on Windows emits the first chunk as `delta`, and Chromium marked it "won't fix". Source: <https://github.com/Vanilagy/mediabunny/issues/365>.
- **FACT.** The WebCodecs bug list maintained by Remotion records false positives from `isConfigSupported`, audio with negative timestamps being ignored, and failures with interlaced video. Source: <https://www.remotion.dev/docs/mediabunny/webcodecs-bugs>.
- **FACT.** In FreeCut, an MP4 export without an AAC encoder available comes out without audio, even with the message "Export complete" (#317). Source: [`projects/freecut.md`](projects/freecut.md).
- **FACT.** FilmCraft wrote its own codecs from scratch in Rust and keeps accumulating issues with real media. Source: [`projects/filmcraft.md`](projects/filmcraft.md).
- **FACT.** Compositions with odd dimensions do not export to H.264 (Remotion #11586, title read).

**Rule.** Do not write codecs. Whether a file can be decoded is verified by actually decoding it, not by asking the API. A single output format in the MVP. Output dimensions are always even. No export finishes "successfully" without the audio track the timeline requires.

## 6. Text, fonts and captions

- **FACT.** Fallback font and different font per operating system: Remotion #11315 and #5984 (above).
- **FACT.** Reframe turns SRT/WebVTT captions into up to 500 `drawtext` filters. Source: [`projects/reframe.md`](projects/reframe.md).
- **FACT (secondary source, not opened).** Report that libass uses the system fontconfig even when `fontsdir` is passed to the `subtitles` filter. Source: <https://community.render.com/t/ffmpeg-libass-subtitles-filter-not-using-fontsdir-parameter-on-render-deployment/39185>.
- **FACT.** Urdu text is not centered in Editly (#161, title read): an indication of missing correct shaping for right-to-left scripts.
- **FACT.** OpenCut stores captions as plain text, and a production product reported five friction points because of it (#898). Source: [`projects/opencut.md`](projects/opencut.md).
- **FACT.** In FreeCut the timeline uses whole frames, but captions use float seconds. Source: [`projects/freecut.md`](projects/freecut.md).

**Rule.** There is a single text layout implementation (line breaking, shaping, metrics), and it feeds both preview and export. A caption is its own item type, with per-word timings in the same integer unit as the rest of the timeline. Fonts travel with the project. The decision about where that single implementation runs is covered in [`stack-tradeoffs.md`](stack-tradeoffs.md).

## 7. Undo and redo

- **FACT.** Olive #1705: undoing the removal of an audio item does not restore the preview, and the error persists after saving. Source: <https://github.com/olive-editor/olive/issues/1705>.
- **FACT.** Olive #2132, #1523, #1781 (titles read): crashes on undo involving transitions and audio nodes.
- **FACT.** Omniclip 2.0: undo via deep-copy snapshots, capped at 64 steps, stored in localStorage; the extracted engine (omnitool) has no undo. Source: [`projects/omniclip.md`](projects/omniclip.md).
- **FACT.** FreeCut: undo (full snapshot) and the agent API (operations dispatcher) are separate layers that duplicate the UI surface. Source: [`projects/freecut.md`](projects/freecut.md).

**Rule.** A single command catalog serves both the UI and the agents, and undo is derived from that same catalog. Every command has a round-trip test (apply, undo, redo returns the identical state). Side effects stay out of the history.

## 8. Project format

- **FACT.** Olive refuses to open an old project and has no migration (#1700, #2229). Source: <https://github.com/olive-editor/olive/issues/1700>.
- **FACT.** OpenShot #5901 (title read): "opening new project overwrites existing project edits".
- **FACT.** The omnitool format has a `version` field, but the skill says `0`, the code writes `1`, and there are no migrators. Source: [`projects/omniclip.md`](projects/omniclip.md).
- **FACT.** voidcut has no versioned schema and no project file. Source: [`projects/voidcut.md`](projects/voidcut.md).
- **FACT.** FilmCraft is at file version 17, migrates one step at a time, and refuses a file newer than the build. Source: [`projects/filmcraft.md`](projects/filmcraft.md).

**Rule.** Schema version from the first commit, chained migrations with a fixture per version, atomic saves, and an explicit refusal (with an actionable message) of a file newer than the binary.

**Rule.** Versions of the schema from the first commit, chained migrations with a fixture per version, atomic saves, and explicit refusal (with an actionable message) of a file newer than the binary.

## 9. Long, stuck or silently wrong export

- **FACT.** OpenCut #914 (stuck at 5% with no error) and FreeCut #317 ("Export complete" with no audio), both above.
- **FACT.** Remotion #10274: wrong frames under concurrency, with no error message ("silently incorrect"); the problem disappears at concurrency 1. Source: <https://github.com/remotion-dev/remotion/issues/10274>.
- **FACT.** The vukem queue is at-least-once and has no progress or per-job cancellation; the existing Studio uses `Retries(0)` so the render does not run twice. Source: [`notes/vukem.md`](notes/vukem.md).

**Rule.** Export is a job with its own state: real progress, cancellation, typed errors, and automatic verification of the final file (streams present, duration, dimensions, `faststart`). An agent never receives "success" for a file that failed this verification. This is the point where the work already done in the Studio (`web/app/studio/qa.go`) is reused.

## 10. Determinism

- **FACT.** Remotion #10274 (above): identical renders produced artifacts in different frames.
- **FACT.** AMD hardware HEVC diverges in the type of the first chunk (mediabunny #365).
- **INFERENCE, no primary source.** Hardware encoders do not produce identical bytes across machines.

**Rule.** Determinism is promised at two levels, and only the first is a contract: (a) the same timeline and the same media produce the same sequence of composed frames, verifiable by hash or by an image metric with a defined tolerance; (b) the final encoded file is not promised byte for byte, except with a software encoder at a fixed version, which is what the reference tests use.

## 11. Licensing

- **FACT.** Remotion: its own license, with a stated effort to enforce it ("enforce licensing - terms, telemetry, and license itself", #9653) and published packages with an inconsistent license declaration (#13403, open as of 2026-11-14). Sources: <https://github.com/remotion-dev/remotion/issues/9653>, <https://github.com/remotion-dev/remotion/issues/13403>.
- **FACT.** FreeCut ships, inside its MIT repository, a copy of SoundTouch, which is LGPL-2.1. Source: [`projects/freecut.md`](projects/freecut.md).
- **FACT.** The author's own Studio app encodes with `libx264` from Alpine's FFmpeg, which is a GPL build. Source: [`notes/existing-studio.md`](notes/existing-studio.md).

**Rule.** Every new dependency comes in with its license verified and recorded. Full treatment in [`licensing.md`](licensing.md).

## 12. Architecture stuck to the UI

This is the post-mortem that matters most for our differentiator.

- **FACT.** OpenCut (98k stars) archived the editor in May 2026 and started over from scratch. The current `main` is only a scaffold; there is no MCP, CLI or API, only third-party requests (#811, #867, #868, #827, #719). Undo was built from non-serializable TypeScript classes, and rendering depends on the DOM, which makes it impossible to run without a browser. Source: [`projects/opencut.md`](projects/opencut.md).
- **FACT.** Omniclip's MCP (`dev` branch) requires an open browser tab; there is no headless render; the CLI is only announced. Source: [`projects/omniclip.md`](projects/omniclip.md).
- **FACT.** FreeCut exposes an agent API, but it launches a Chrome via Playwright and duplicates the UI operations in another dispatcher. Source: [`projects/freecut.md`](projects/freecut.md).
- **FACT.** FilmCraft is the counterexample: it was born with a single command registry, dispatched by UI, CLI and MCP. Source: [`projects/filmcraft.md`](projects/filmcraft.md).

**Rule.** The command core and the render must work without a UI and without an open tab from day one. Those who tried to add this later had to either rewrite it or wrap a browser around it.

## 13. Gaps

- Several issues had only their title read: Olive #2132, #1523, #1781; OpenShot #5869, #5901, #5392; Shotcut #1774, #1517; Editly #72, #117, #161; Remotion #11586, #12927; OpenCut #832, #912, #915. They are marked in the text and do not, on their own, support any rule.
- Not covered: the Kdenlive bugtracker and the engineering blogs of Clipchamp, Canva, Descript, Kapwing and Veed.
