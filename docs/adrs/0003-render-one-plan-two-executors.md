# ADR-0003: Render: one frame plan, two executors, truth on pause

- Status: Accepted (2026-11-14); items 3 and 4 amended by [ADR-0029](0029-one-renderer-webgpu-webcodecs-rust.md) (2026-11-16)
- Deciders: author (D-001), architect
- Links: stack trade-offs §4, post-mortems §1, §6, §10, spikes, SDD-004

## Context

The briefing asks for "a single render path as the source of truth for preview and export" and for a precise definition of what that means and what difference is tolerated. D-001 fixes FFmpeg, driven by Go, as the export and the truth. The risk is the browser preview becoming a second renderer: FreeCut and Reframe both show preview and export diverging because the preview is DOM/CSS and the export is something else; Remotion documents differences even between its own two renderers.

## Decision

"Single path" means: **one function decides what every frame contains; two executors only put pixels where it says.**

1. **Frame plan.** `core/plan` has one function that, for a project revision and a frame number, returns the ordered layers: source media and source frame, crop, affine transform, opacity, and the bitmaps to draw with their positions. Keyframe interpolation, easing, transition timing and source-frame selection exist only in this function.
2. **Rasterizer.** Text and shapes become RGBA bitmaps in the Rust rasterizer, `renderer/vukem-renderer-text`, reached from Go through `engine/rasterize` (ADR-0012; it was Go code in `engine/raster` until TASK-0220). FFmpeg's `drawtext`, `subtitles` and libass are not used.
3. **Export executor.** `engine/export` composes every frame in Go: FFmpeg decodes the normalized media into raw frames, a Go compositor places the layers and bitmaps exactly as the plan says, a Go mixer builds the audio, and FFmpeg encodes the result. FFmpeg places nothing. The first spike showed that placing a layer through FFmpeg `overlay` expressions produced frames that differed from the piped version in 630 of 900 frames, and the LGPL build lacks the `eq` and `boxblur` filters; composing in Go removes both dependencies. The design and its performance fallbacks are in SDD-004 §6 and §12.
4. **Preview executor.** The browser plays proxies in `<video>` elements and composites them with the same bitmaps on a canvas, following plan windows fetched from the core (ADR-0013). It interpolates nothing.
5. **Truth on pause.** When playback stops on a frame, the UI shows that frame rendered by the export executor at preview resolution. The agent's `view` tool uses the same endpoint. What a person inspects while paused and what an agent sees is what will be exported.
6. **Tolerance.** Between the preview executor during playback and the export executor, a frame may differ by scaling filter, colour conversion and blend rounding. The accepted difference is: SSIM of at least 0.97 on the luma plane for every reference frame at preview resolution, and no layer displaced by more than 1 pixel at preview resolution. Between two runs of the export executor on the same machine and FFmpeg build, composed frames before encoding are identical. Encoded files are not promised to be byte-identical across machines (ADR-0011).

## Alternatives

- **Independent browser renderer for preview, FFmpeg for export.** Rejected: two implementations of layout and text.
- **Browser renderer as the only renderer, headless Chrome for export.** Rejected by D-001.
- **Engine-rendered preview streamed to the browser for everything.** Rejected as the only mode: every drag would wait for a render. Kept for the paused frame.

## Consequences

Positive: the semantics of a frame exist once and are testable without pixels (the plan is data). Text is identical in both executors because both draw the same bitmap.

Positive: the browser needs only `<video>`, a WebGL2 canvas and server-sent events, which lowers the bar for a desktop webview and for other browsers.

Negative: visual effects are limited to what the plan and our rasterizer can express. Accepted by the author in D-001.

Negative: two pixel executors still exist, so reference-frame tests comparing them are mandatory and must run in CI.

Negative: the paused exact frame costs an FFmpeg run; it needs a cache keyed by project revision and frame.
