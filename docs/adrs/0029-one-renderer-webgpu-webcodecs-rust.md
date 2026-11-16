# ADR-0029: One frame plan, one renderer, two hosts: WebCodecs, WebGPU and a Rust renderer in WebAssembly

- Status: Accepted (2026-11-16)
- Deciders: author (D-035), orchestrator
- Amends: ADR-0003 items 3 and 4 (the export executor and the preview executor); AGENTS.md rule 5 ("pixels are composed in Go")
- Links: [brainstorming D-035](../brainstorming.md), ADR-0002, ADR-0003, ADR-0007, ADR-0011, ADR-0012, ADR-0013, SDD-004, SDD-012, SDD-013 (to be written by P5-RND-001)

## Context

ADR-0003 settled "one function decides what every frame contains; two executors only put pixels where it says". The plan (`core/plan`) is in Go and stays the single decision. The two executors are different code: the export composes in Go (`engine/compositor`, `engine/raster`) and the browser preview composes with PixiJS on WebGL over `<video>` elements. BETA spent a large part of its effort keeping them within the tolerance of ADR-0003 item 6 (SSIM of at least 0.97, at most 1 pixel): `preview-vs-export` carried expected divergences until v0.17.0, and an intermittent black video texture is still open (P2-QA-003).

The author wants the product to grow into the main open-source alternative to CapCut. That needs real-time effects, many simultaneous video layers, frame-exact scrubbing, and later mobile and offline browser editing. The CPU compositor in Go and one `<video>` element per layer will not carry that, and two independent executors make every new effect a second implementation.

## Decision

**One frame plan (Go), one renderer (Rust), two hosts (the browser and the Go export).** Each technology has one bounded place.

1. **The plan does not move.** `core/plan` (Go) stays the only place that decides layers, source frames, transforms, keyframe interpolation, easing, transitions and timing. The renderer receives a frame plan and draws it; it never interpolates, never reads the document and never decides anything an agent could see differently from a person (AGENTS.md rule 1).
2. **Rust only in the renderer.** A Cargo workspace under `renderer/` holds the pixel renderer: composition (blend, affine transform, crop, opacity, colour adjust, blur and filters, transitions as the plan describes them), then the text and caption rasterizer that `engine/raster` does today. No other part of the product is written in Rust.
3. **Two render paths inside the renderer, one truth.**
   - **CPU reference path:** integer or fixed-point arithmetic, no platform SIMD differences, bit-exact on Linux, macOS and Windows and in WebAssembly. It is the truth: golden frames are byte-for-byte against it, as they are against the Go compositor today.
   - **GPU path (`wgpu`):** WebGPU in the browser, Vulkan, Metal or D3D12 natively. It is held to the CPU path by a strict tolerance (SSIM of at least 0.99 on luma, at most 0.5 pixel), tighter than ADR-0003 item 6 because the code is shared.
4. **WebAssembly carries the renderer to both hosts.** The crate compiles to `wasm32-unknown-unknown` (`wasm-bindgen`).
   - **Browser host:** the preview loads the module in a worker, draws with the GPU path on WebGPU, and falls back to the CPU path on a 2D canvas when WebGPU is missing.
   - **Go host:** the export runs the same module's CPU path through `wazero` (pure Go, no cgo), so cross-compiled pure-Go builds stay as they are and the module runs sandboxed in process, with no file, network or clock access.
   - **Native GPU host (later, optional):** a `vukem-render` binary using the GPU path natively, run as a child process under the process policy of SDD-007 §6 like FFmpeg, for faster 4K export where a GPU exists. It is never the default until it passes the parity gates, and the CPU path stays the fallback.
5. **WebCodecs decodes the preview's video.** A worker decodes video layers with `VideoDecoder` from the proxies the engine already writes, using a chunk index the engine adds next to each proxy (codec string, sample sizes and offsets, keyframes), so the browser needs no demuxer library. Seeking is frame-exact. Each decoded `VideoFrame` goes to the renderer as a texture and is closed after use. `<video>` stays as the fallback when the codec configuration is not supported.
6. **WebGPU arrives in two steps.** First, PixiJS 8's WebGPU renderer replaces WebGL in the current preview, with WebGL as the fallback (no renderer change, quick). Then the Rust renderer replaces PixiJS in the preview.
7. **Truth on pause stays.** The paused frame is still the export's frame (ADR-0003 item 5). With one renderer it becomes the same code as the playing preview, so the switch on pause should show no visible change.
8. **Staged, flag-gated.** Each stage ships behind a setting or feature flag, off by default until its gates pass on the three operating systems and the three browsers. The Go compositor and the PixiJS preview stay until the Rust paths pass the gates, then they are removed in the same release that makes the new path the default.

## Stages

| Stage | What | Gate to make it default |
| --- | --- | --- |
| A | PixiJS 8 on WebGPU with WebGL fallback | `preview-vs-export` 100% on chromium, firefox, webkit; no new CSP violation |
| B | WebCodecs decode in a worker, chunk index from the engine, `<video>` fallback | frame-exact seek; P2-QA-003 black texture gone in 500 repeated cases; NFR-003 and NFR-004 on a quiet machine |
| C | Rust renderer, CPU path, run by the Go export through wazero | every golden frame byte-equal to the Go compositor; export speed within 20% of the Go compositor; NFR-007 memory |
| D | The same module, GPU path on WebGPU, as the browser preview | SSIM of at least 0.99 and 0.5 pixel against the CPU path on every golden case in three browsers; NFR-001, NFR-003, NFR-004 |
| E | Text and caption rasterizer moved into the renderer | raster goldens byte-equal; `TestBundledFontsHaveLicence`; user fonts |
| F (optional) | Native `vukem-render` GPU export host | parity with the CPU path as in D; faster 4K export measured; process policy tests |

## Alternatives

- **Keep the BETA stack** (Go compositor, PixiJS on WebGL, `<video>`). It is correct today, but it does not scale to real-time effects and many layers, and every effect stays a double implementation.
- **The whole engine in Rust, command core included.** Rejected: the command core is not a performance problem (`Apply` under 5 ms), it is the most tested part (96 to 98% coverage), and a second core would break AGENTS.md rule 1.
- **Rust linked into Go with cgo.** Rejected: it breaks the pure-Go, cross-compiled builds of every platform and the `CGO_ENABLED=0` desktop builds.
- **GPU-only export.** Rejected: floating-point results differ across GPU vendors and drivers, so golden frames could not be byte-exact.
- **Go compiled to WebAssembly for the browser.** Rejected: large modules, no direct WebGPU path, and the compositor is the part that needs SIMD and GPU.

## Consequences

- A second toolchain (Rust stable, pinned) joins the build: CI, notices and the licence gate must cover crates (`cargo-deny` with the same allowlist as `internal/tools/notices`).
- The WebAssembly module is a build artifact embedded in the Go binary and served to the browser, so it is reproducible and checked by digest like the other pinned inputs.
- `wazero` executes untrusted-free code (our own module) but receives untrusted media frames; the module has no host imports beyond memory, so it cannot reach files, network or time (SDD-007 gains a threat row).
- ADR-0003 item 6's tolerance (0.97, 1 pixel) stays for stages A and B; from stage D the preview-to-export tolerance becomes the GPU-to-CPU tolerance (0.99, 0.5 pixel).
- The ROADMAP gains Phase 5 (Renderer evolution). It may run in parallel with what remains of Phase 4, because its files do not intersect Phase 4's and every stage is flag-gated.
