# Performance: where the time and the memory go

> Stage 1, research. All sources accessed 2026-11-14.
> Marking: **FACT**, **INFERENCE**, **OPINION** (the architect's judgement), **NOT VERIFIED**.
> Evidence: [`notes/stack-browser.md`](notes/stack-browser.md) (parts A and C), [`notes/engine-postmortems.md`](notes/engine-postmortems.md), and the measurements in [`spikes.md`](spikes.md).

## Contents

1. [What the architecture already avoids](#1-what-the-architecture-already-avoids)
2. [Memory](#2-memory)
3. [Scrubbing and playback in the browser](#3-scrubbing-and-playback-in-the-browser)
4. [Export speed](#4-export-speed)
5. [Timeline size](#5-timeline-size)
6. [Hardware acceleration and variable frame rate](#6-hardware-acceleration-and-variable-frame-rate)
7. [Proposed budgets](#7-proposed-budgets)
8. [Gaps](#8-gaps)

## 1. What the architecture already avoids

Most performance post-mortems of browser editors come from doing the heavy work inside a tab. The chosen design (see [`stack-tradeoffs.md`](stack-tradeoffs.md) §4) moves that work out.

| Known problem | Evidence | Why it does not apply |
| --- | --- | --- |
| Export assembled in one memory buffer fails on long timelines | OpenCut #914 ("Array buffer allocation failed"); FreeCut #316 | Export is an FFmpeg process writing to disk |
| ffmpeg.wasm cannot handle files above 2 GB | ffmpeg.wasm #876 | Not used |
| `VideoFrame` objects not closed leak GPU memory | mediabunny #144; the WebCodecs specification says resources "SHOULD be released immediately" | The MVP does not use WebCodecs |
| Encoding slows when the page re-renders or the tab is in the background | Chromium issue 414491381; Remotion's client-side limitations page | Encoding is not in the page |
| Linux Chromium builds may lack an H.264 encoder | IMG.LY's limitations page | Encoding is not in the browser |

The problems that remain are the ones below.

## 2. Memory

- **INFERENCE (arithmetic).** One 1080×1920 RGBA frame is 8.3 MB. A full-frame overlay piped to FFmpeg at 30 fps is about 250 MB per second of raw data crossing a pipe.
- **FACT.** A decoded 4K frame is 33 MB, about 1 GB per second at 30 fps. Source: <https://www.descript.com/blog/article/the-new-descript-how-we-multiplied-the-apps-speed-and-performance>.

**What this changes.** The Go process must never hold a clip's frames in memory; it streams. Overlay bitmaps are sent only when they change, or as a cropped region instead of the full frame. The spike measures the three ways of feeding overlays to FFmpeg.

In the browser, memory is bounded by the number of `<video>` elements kept alive and by cached bitmaps. Both get explicit caps.

## 3. Scrubbing and playback in the browser

- **FACT.** Descript on the limits of `<video>`: efficient for playback, limited for frame-accurate stepping and for audio and video sync across layers. It transcodes media on the server into a consistent format and streams small chunks. Same source as above.
- **NOT VERIFIED.** No primary source was found on whether a `<video>` seek lands on the exact frame or on a keyframe.
- **NOT VERIFIED.** No published limit on simultaneous hardware video decoders in desktop Chrome was found.
- **INFERENCE.** A seek in long-GOP footage decodes from the previous keyframe, so phone footage scrubs badly; a proxy with a short GOP scrubs well.

**What this changes.**

1. The browser plays proxies made at ingest: constant frame rate, short GOP, reduced resolution. It never plays the original.
2. Coarse scrubbing over the timeline uses thumbnail strips generated at ingest.
3. Frame accuracy is not asked of `<video>`. When playback stops, the exact frame comes from the export executor ("truth on pause").
4. The number of video layers decoded at the same time in the preview is capped; layers beyond the cap show a still until they become the focus. The cap is a measured number, set in the Prototype.
5. The preview clock follows the audio.

## 4. Export speed

The reference numbers come from the spike, not from this document: see [`spikes.md`](spikes.md).

- **INFERENCE.** Speed depends on which H.264 encoder the machine has (D-006), so it varies by machine. Hardware and operating system encoders are fast; OpenH264 on a CPU-only server is the slow case.
- **INFERENCE.** The filter graph is built per export from the frame plan. Graphs with hundreds of inputs are slow to build and heavy in memory (NOT VERIFIED, from the research notes), so long timelines are rendered in segments with closed GOPs and concatenated. Segments also give real progress, cancellation and resume ([`post-mortems.md`](post-mortems.md) §9).

## 5. Timeline size

- The product targets videos of 30 seconds to 5 minutes. At 30 fps, 5 minutes is 9,000 frames.
- **INFERENCE.** A timeline of that length has tens of clips and, with word-level captions at about 150 words per minute, around 750 caption words. A DOM timeline handles that without virtualization tricks; a canvas timeline is not needed.
- **INFERENCE.** The frame plan is fetched by the browser in windows of a few seconds, not for the whole timeline, and cached by project revision.
- **INFERENCE.** The compact text view of the timeline for agents is one line per item. Captions are summarized as one line per caption item with its word count, with the words available on request, so a caption-heavy project does not flood an agent's context.

## 6. Hardware acceleration and variable frame rate

- **FACT.** Recordings from hardware encoders at a nominal 30 fps have real intervals between 29.4 and 30.3 fps (OBS #13396).
- **INFERENCE.** Normalizing to constant frame rate at ingest costs one transcode per imported file and removes frame-selection bugs from every later step. For a product whose inputs are phone and screen recordings this is the right trade.
- **INFERENCE.** Hardware decoding is used by the browser for proxies without our involvement. Hardware encoding is a matter of which FFmpeg encoder is selected, with a defined fallback order.

## 7. Proposed budgets

These are the architect's proposals (OPINION) for the non-functional requirements of the PRD. They are deliberately stated as numbers so they can be measured, and each is confirmed or corrected by a measurement in the Prototype on a named reference machine.

| Measure | Budget |
| --- | --- |
| Command round trip (request to state applied and returned), local, 95th percentile | 50 ms |
| A project event reaching the UI after a command from another actor | 200 ms |
| Proxy frame shown after a seek while scrubbing | 100 ms |
| Exact frame shown after pausing, at preview resolution | 500 ms |
| Ingest of a 60 s 1080p phone clip (probe, normalized media, proxy, thumbnails, peaks) | 30 s |
| Export of a 60 s 1080×1920 30 fps project with captions, with a hardware or operating system encoder | 60 s |
| Go process resident memory during an export | 500 MB |
| Browser tab memory with a 5 minute project open | 1 GB |
| Supported project: duration, items, caption words | 5 min, 200 items, 2,000 words |
| Frame or contact sheet returned to an agent | 2 s |

## 8. Gaps

- No public benchmark exists for a 1080×1920, 5 minute export in any pipeline comparable to this one.
- Limits of simultaneous decoders and `<video>` seek precision are unverified and need measurement.
- The budgets above have not been measured.
