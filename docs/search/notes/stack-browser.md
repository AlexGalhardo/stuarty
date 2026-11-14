# Browser stack: platform APIs and libraries (media, preview, timeline)

> **Status of this file.** Research notes delivered by a research agent on 2026-11-14 and reviewed by the architect. The opinions and recommendations in this file predate the author's decisions (`../../brainstorming.md`, D-001 to D-016) and the ADRs (`../../adrs/`). Where they differ, the ADRs prevail. Facts rechecked by the architect are listed in [`architect-verifications.md`](architect-verifications.md).

Research done on 2026-11-14 (all sources "accessed 2026-11-14"). Labels: FACT (seen in the cited primary source), INFERENCE (my reasoning, explained), OPINION.
Release date convention: "date" = the release's `published_at` on GitHub when it exists, otherwise the npm `time.modified` (approximation, indicated).

## Index

- Part A: platform APIs (A1 to A15) + desktop webview matrix
- Part B: library table (B1 to B8) + license notes
- Part C: answers to the 3 questions
- Gaps and what was NOT verified

Version reference: Chrome stable Windows = 158.0.8127.37 (<https://versionhistory.googleapis.com/v1/chrome/platforms/win/channels/stable/versions?pageSize=1>, accessed 2026-11-14). FACT. WebView2 Fixed Runtime listed on the download page: 157.0.4341.60 (<https://developer.microsoft.com/en-us/microsoft-edge/webview2/>, accessed 2026-11-14). FACT.

---

## PART A: Platform APIs

### A1. WebCodecs

- Interfaces: VideoDecoder/VideoEncoder/AudioDecoder/AudioEncoder, VideoFrame, AudioData, EncodedVideoChunk/EncodedAudioChunk, ImageDecoder. Available in Dedicated Workers. FACT (<https://developer.mozilla.org/en-US/docs/Web/API/WebCodecs_API>).
- Encodes/decodes only: it does not read containers or write files (it needs a demuxer/muxer). A full codec string is required (`avc1.4d0034`, `vp09.00.40.08.00`). FACT (MDN, same URL).
- Codecs (MDN): H.264 is the broadest; VP9 is common; AV1 has broad decode and limited hardware encode; HEVC "with significant gaps outside Apple platforms"; Opus recommended; AAC broad; MP3 decode only; PCM. FACT (MDN). The exact matrix per platform was NOT verified: MDN points to an external "Codec Support Table" (webcodecsfundamentals.org), which did not open with useful content.
- AAC encode: "not supported in some browsers"; Mediabunny offers the polyfill `@mediabunny/aac-encoder` (WASM, 5.4 MB unpacked). Same for MP3/FLAC (polyfills); AC-3/DTS/TrueHD/ProRes do not exist in WebCodecs. FACT (<https://mediabunny.dev/guide/supported-formats-and-codecs>; npm `@mediabunny/aac-encoder` unpackedSize 5385642).
- Support per browser (caniuse): VideoEncoder/VideoDecoder: Chrome/Edge 94+, Firefox 130+, Safari 16.4+ (video only); Audio encoder/decoder in Safari arrived in 26.0 ("Partial" in 16.4-18.7, "complete" 26.0+); Chrome Android 157; Firefox Android NOT. FACT (<https://caniuse.com/webcodecs>, <https://caniuse.com/mdn-api_videoencoder>; WebKit confirms AudioEncoder/AudioDecoder in Safari 26.0: <https://webkit.org/blog/17333/webkit-features-in-safari-26-0/>).
- Safari 26.6/27.0 limitation (macOS): AudioDecoder.isConfigSupported returns false for 3 to 16 channels (mono/stereo OK), inconsistent with MediaCapabilities. FACT (<https://bugs.webkit.org/show_bug.cgi?id=327697>).
- AudioData does not integrate directly with Web Audio: it needs `copyTo()` into a Float32Array and building an AudioBuffer. Frequent `flush()` worsens encoder quality and forces a keyframe in the decoder. FACT (MDN).
- Codec reclaim: Chrome 94+ reclaims inactive decoders/encoders after 1+ min by calling `error()` with QuotaExceededError; a more aggressive reclaim in background tabs is planned. FACT (search result citing Chrome 94 release notes; secondary source, original text not opened). The spec defines "codec saturation" (queue-based backpressure): <https://www.w3.org/TR/webcodecs/>.
- Simultaneous decoder limit: I did not find a published limit for Chrome desktop. I only saw the ChromeOS VA-API limit (16, 10 on Stoney Ridge): <https://chromium.googlesource.com/chromium/src/+/c74bb01a2889afdfdd1e06f0144fb7b02e2e00a2>. FACT for ChromeOS; for desktop it is a GAP. INFERENCE: treat it as a limit of 2 to 4 active HW video decoders and use a pool with LRU (a defensive practice, not a measured figure).
- Memory: a decoded 4K frame = 33 MB, ~1 GB/s at 30 fps; not calling `close()` leaks GPU memory. FACT (Descript: <https://www.descript.com/blog/article/the-new-descript-how-we-multiplied-the-apps-speed-and-performance>) / practice (secondary source about close()).
- Linux: Chromium says VA-API on Linux is "not officially supported", only via flags (VaapiVideoDecodeLinuxGL, VaapiVideoEncoder), and NVIDIA requires a third-party driver: <https://chromium.googlesource.com/chromium/src/+/HEAD/docs/gpu/vaapi.md>. FACT. I did not find current docs confirming HW H.264 encode on Linux. Open-source Chromium builds on Linux may lack an H.264 encoder: <https://img.ly/docs/cesdk/js/create-video/limitations-6a740d/> (FACT via IMG.LY). Detection: `VideoEncoder.isConfigSupported({hardwareAcceleration:'prefer-hardware'})`. INFERENCE: Google's official Chrome on Linux has H.264 in software (OpenH264), but I did not verify it; treat "client-side export on Linux needs a server fallback" as the working assumption.
- WHAT CHANGES IN THE ARCHITECTURE: WebCodecs is viable as the decode engine for preview and the encode engine for export in Chrome. It requires (1) a feature-detection layer per codec/machine with a fallback (FFmpeg server), (2) a VideoFrame lifecycle policy (always close), (3) a decoder pool, (4) muxing via a library (Mediabunny).

### A2. WebGPU

- Platform status (Chromium): Windows/macOS/ChromeOS since 113; Android 12+ (Qualcomm/ARM/Intel) since 121; Linux: Intel Gen12+ since 144, NVIDIA (driver 535.183.01+) on Wayland since 147, the rest behind a flag; Windows ARM64 behind a flag. FACT (<https://github.com/gpuweb/gpuweb/wiki/Implementation-Status>).
- Firefox: Windows 141, macOS Apple Silicon 145 (macOS 26+) / 147 (all), Linux only Nightly, Android behind a flag. Safari 26+ on macOS/iOS/iPadOS/visionOS. FACT (same URL; caniuse <https://caniuse.com/webgpu> shows Safari desktop "partial" from 26.0 to 27.3 TP).
- Importing a VideoFrame as an external texture: `device.importExternalTexture({source: VideoFrame | HTMLVideoElement})`; the texture from a VideoFrame lives until `frame.close()`, the one from `<video>` expires on first use; available in Workers; marked "limited availability". FACT (<https://developer.mozilla.org/en-US/docs/Web/API/GPUDevice/importExternalTexture>).
- Architecture change: with WebGPU the path decode(HW) -> VideoFrame -> external texture -> composition -> VideoFrame -> encode is zero-copy. But Linux/Firefox/Android coverage is still partial; WebGL2 should be the compositor fallback (A3). For v1, stable Chrome only: WebGPU on Windows/macOS is safe; Linux depends on GPU/driver.

### A3. WebGL2 as fallback

- Universally available in modern browsers (INFERENCE, I did not search the table). A VideoFrame can be passed to `texImage2D` (copy/upload; there is no zero-copy "external texture" path as in WebGPU) (general knowledge, NOT verified in this session).
- Tauri documents that on Linux (WebKitGTK) the DMABUF renderer conflicts with NVIDIA and recommends a fallback without WebGL: <https://v2.tauri.app/develop/debug/linux-graphics/>. FACT.
- Change: keep the compositor behind an interface (abstract renderer) with WebGPU and WebGL2 backends (e.g., PixiJS v8 already abstracts both, see B).

### A4. OffscreenCanvas and Workers

- WebCodecs and `importExternalTexture` work in Workers (MDN above). OffscreenCanvas in a Worker lets you compose/encode off the main thread. Remotion client-side runs on the main thread, "without multithreading", and aborts slowly in a background tab (rAF throttle; Web Worker timer fallback): <https://www.remotion.dev/docs/client-side-rendering/limitations>. FACT.
- Change: recommended architecture = render Worker (OffscreenCanvas + WebGPU + WebCodecs + Mediabunny), UI on the main thread. An export in a background tab is throttled if it depends on rAF; a render in a Worker without rAF avoids this.

### A5. OPFS and quotas

- FileSystemSyncAccessHandle: Chrome/Edge 102+, Firefox 111+, Safari 15.2+ (iOS 15.2+), Chrome Android 157. FACT (<https://caniuse.com/mdn-api_filesystemsyncaccesshandle>). SyncAccessHandle only in a Worker (general knowledge, not re-verified).
- Quotas: Chromium up to 60% of the disk per origin (same persistent limit); Firefox min(10% disk, 10 GiB) best-effort, persistent up to 50%; Safari ~60% of the disk per origin in a browser app, ~15% for embedded WebKit apps that are not browsers (relevant for WKWebView!). Writing above quota throws QuotaExceededError; eviction is per entire origin (LRU). FACT (<https://developer.mozilla.org/en-US/docs/Web/API/Storage_API/Storage_quotas_and_eviction_criteria>).
- Change: OPFS is the place for cache (proxies, thumbnails, waveforms, streaming export output). Request `navigator.storage.persist()`. On desktop via WKWebView the quota is smaller (~15%, FACT above), and project data must live on real disk (via the native host), not only in OPFS.

### A6. File System Access API (local files without copying)

- `showOpenFilePicker` etc.: Chrome/Edge 105+, Opera 91+; does NOT exist in Safari or Firefox; Chrome Android 157 is listed as unsupported. FACT (<https://caniuse.com/native-filesystem-api>).
- Change: in Chrome v1 you can store a `FileSystemFileHandle` (persistable in IndexedDB, requires re-permission) and read via `File.slice`/streams without copying to OPFS. Safari/Firefox fallback = `<input type=file>` (a File without a persistent handle) or a copy to OPFS. The cost of supporting Safari/Firefox later: a "media source" layer with two implementations (persistent handle vs OPFS copy) and relink UX.

### A7. Memory limits (tab, ArrayBuffer, WASM, memory64)

- Memory64 (64-bit WASM): Chrome 133 and Firefox 134 shipped it (SpiderMonkey blog: <https://spidermonkey.dev/blog/2025/01/15/is-memory64-actually-worth-using.html>, FACT; the blog also warns that memory64 can be slower). Safari: I did not confirm it shipped; a WebKit bug open since Oct/2025 said it was the only missing WASM feature (<https://bugs.webkit.org/show_bug.cgi?id=300538>). GAP: no confirmation of stable Safari.
- Exact limits per tab (V8 heap, maximum ArrayBuffer, WASM 4 GB without memory64): I did not verify them in a primary source this session. GAP. Related data: CE.SDK (IMG.LY) documents an engine with up to 4 GB on desktop and 2 GB on mobile: <https://img.ly/docs/cesdk/js/create-video/limitations-6a740d/> (vendor FACT).
- Change: never materialize a whole video in an ArrayBuffer; use streams/chunks and mux in streaming to OPFS/FileSystemWritableFileStream. ffmpeg.wasm (B) suffers from this (it loads files into the in-memory FS).

### A8. requestVideoFrameCallback (rVFC)

- Chrome/Edge 83+, Firefox 132+, Safari 15.4+. FACT (<https://caniuse.com/mdn-api_htmlvideoelement_requestvideoframecallback>).
- `metadata.mediaTime` carries the PTS of the presented frame; `currentTime` in Chromium is based on the audio clock. FACT (summary of <https://web.dev/articles/requestvideoframecallback-rvfc>).
- Change: useful to sync overlays with `<video>` in simple previews, but it does not solve precise scrubbing or multi-layer.

### A9. Web Audio / AudioWorklet (mixing and sync)

- I did NOT research a primary source for support (AudioWorklet is broad in Chrome/Safari/Firefox; training knowledge, not verified). FACT that matters: Descript uses WebAudio for playback and libav.js (WASM) for audio decode "because not all browsers support WebCodecs for audio" (<https://www.descript.com/blog/article/the-new-descript-how-we-multiplied-the-apps-speed-and-performance>). Remotion client-side mixes captured audio and encodes via Mediabunny (<https://www.remotion.dev/docs/client-side-rendering/how-it-works>).
- Change: preview master clock = AudioContext.currentTime; video "follows" the audio. Export mixing in OfflineAudioContext or a manual mix by Float32Array, then AudioEncoder (Opus/AAC; AAC may require a polyfill, A1). Chrome caveat: concurrent audio decode limited to 1 (report in WebKit bug <https://bugs.webkit.org/show_bug.cgi?id=284448>, secondary and old source).

### A10. `<video>` for preview and seek precision

- Verified fact: rVFC gives `mediaTime`, and `currentTime` is not guaranteed frame-accurate (community; no spec source found). Descript: "HTML video elements are efficient for playback, but limited in frame-accurate stepping and A/V sync control across layers" (<https://www.descript.com/blog/article/the-new-descript-how-we-multiplied-the-apps-speed-and-performance>). FACT of the vendor.
- GAP: I did not find a primary source (spec/bug tracker) on whether seek lands on a keyframe or an exact frame. Training knowledge says `<video>` seek in Chromium tends to be frame-accurate for normal MP4 but varies across browsers/containers; do NOT use it as truth without a test.
- Change: `<video>` serves single-clip preview and simple reading; a multi-layer timeline with A/V sync and frame stepping needs WebCodecs.

### A11. HTMLVideoElement vs WebCodecs for scrubbing

See Part C3.

### A12. SharedArrayBuffer and COOP/COEP

- SAB only with `crossOriginIsolated`: `Cross-Origin-Opener-Policy: same-origin` + `Cross-Origin-Embedder-Policy: require-corp|credentialless`. COOP cuts the relation with popups/openers; cross-origin resources need CORS/CORP. FACT (<https://developer.mozilla.org/en-US/docs/Web/API/Window/crossOriginIsolated>).
- Change: only needed for multi-threaded ffmpeg.wasm (`@ffmpeg/core-mt`) or your own WASM threads. The WebCodecs+Mediabunny pipeline does not require SAB. Avoiding COOP/COEP simplifies embeds, OAuth popups and CDN assets. Decision: do not depend on SAB in v1. In a desktop webview, the headers must be served by the app's custom protocol.

### A13. VFR (variable frame rate)

- GAP: I did not find a primary source on how the browser handles VFR. Fact about Mediabunny: the API is based on microsecond timestamps (`getSample(t)` returns the last frame with ts <= t), which models VFR naturally (<https://mediabunny.dev/guide/media-sinks>, FACT). Remotion/Revideo require a constant frame rate in the composition (general knowledge, not verified). Change: the project's time model should be rational time/ts in microseconds, with optional normalization to CFR on export.

### A14. WebTransport / WebSocket / SSE

- I did NOT research support this time (outside the critical media path). WebSocket/SSE are universal (general knowledge). For AI agents: the natural channel is commands over HTTP/WebSocket/SSE for the same "project document" (see C2). WebTransport is not needed in v1. Marked OPINION.

### A15. Desktop webviews (decides "same UI via the system webview")

| Webview | Engine | WebCodecs | WebGPU | OPFS | Source / status |
|---|---|---|---|---|---|
| WebView2 (Windows) | Evergreen Chromium (fixed runtime 157.0.4341.60 listed; Chrome stable 158) | Expected same as Chrome | Expected same as Chrome (Windows since 113) | Expected same as Chrome | Evergreen confirmed (FACT <https://developer.microsoft.com/en-us/microsoft-edge/webview2/>); feature parity with Chrome = INFERENCE (same engine), "feature availability may vary by device type, market, browser version" (FACT, same page). Not tested |
| WKWebView (macOS) | System WebKit/Safari | Video since Safari 16.4; audio since 26.0; >2-channel audio decode bug (26.6/27.0) | Safari 26+ on macOS | Safari 15.2+ | FACT for Safari (links in A1/A2/A5). That WKWebView exposes the same set as Safari = INFERENCE NOT VERIFIED (the search found no source; the Safari 26.0 blog only mentions WKWebView in other contexts). Storage quota for non-browser WebKit apps ~15% (FACT MDN) |
| WebKitGTK (Linux) | WebKit with GStreamer | Mentioned in the Igalia newsletter as part of the GStreamer multimedia support ("playback, capture, WebAudio, WebCodecs, WebRTC"), with no maturity detail | Nothing found in the 2.52 notes | Nothing found | Limited FACT: <https://blogs.igalia.com/webkit/blog/2026/wip-58/> and <https://webkitgtk.org/2026/03/18/webkitgtk-2.52-highlights.html> (neither mentions WebCodecs/WebGPU/OPFS). Real problems: WebRTC absent in the default build, an `ended` bug, DMABUF/NVIDIA (<https://v2.tauri.app/develop/debug/linux-graphics/>, <https://wails.io/docs/guides/linux>). |

Conclusion (INFERENCE + OPINION): "same UI via the system webview" is NOT safe with the WebCodecs+WebGPU pipeline on Linux (WebKitGTK with no evidence of usable WebCodecs/WebGPU) and is uncertain on macOS (WKWebView). Windows via WebView2 is essentially Chrome. Options: (a) desktop = packaged Electron/Chromium (the same engine on all OSes, at a cost of ~100+ MB and updates); (b) desktop = webview + native render/decode via the host's FFmpeg (the UI only edits and previews with proxies via `<video>`); (c) v1 web Chrome only, with desktop later. I recommend deciding after a 1-day spike that runs `isConfigSupported` for VideoEncoder/Decoder/AudioEncoder and `navigator.gpu` inside real WKWebView and WebKitGTK (not done here).

---

## PART B: Candidate libraries

Columns: name · function · latest release (version + date) · last commit (pushed_at) · license · maturity (stars, open issues, adoption) · risks · verdict · source+date.
Standard sources: `npm view <pkg>` and `gh api repos/<repo>` on 2026-11-14 (registry.npmjs.org / api.github.com). Issues = `open_issues_count` (includes PRs).

### B1. Media: demux/mux/codec/FFmpeg

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source |
|---|---|---|---|---|---|---|---|---|
| mediabunny | Demux/mux (MP4, MOV, WebM, MKV, HLS, WAVE, MP3, Ogg, ADTS, FLAC, TS), WebCodecs wrapper, Conversion API, sinks (frames/canvas), 25+ codecs, zero dependencies, tree-shakable (5 kB gz minimum), streaming I/O | 1.66.0, 2026-11-13 | 2026-11-14 | MPL-2.0 (use in closed-source is OK; modified files distributed stay under MPL) | 7,498 stars, 60 issues; base of Remotion client-side and of @diffusionstudio/core v4; replaces mp4-muxer/webm-muxer (deprecation README) | Almost a single maintainer (Vanilagy), funded by sponsors; MPL-2.0 requires publishing changes to MPL files; no 2.x release yet; ProRes/AC-3 etc. only via extra packages | ADOPT | <https://github.com/Vanilagy/mediabunny>, <https://mediabunny.dev>, README via raw.githubusercontent, npm (2026-11-14) |
| @mediabunny/server | Mediabunny in Node/Bun/Deno (backend) | 1.66.0, 2026-11-13 | (monorepo) | MPL-2.0 | same org | Separate package, less tested server use (INFERENCE) | EVALUATE (useful for the server-side export worker without FFmpeg, for simple cases) | npm 2026-11-14 |
| mp4box.js | MP4 demux/mux (GPAC) | 2.5.0, 2026-10-25 | 2026-11-05 | BSD-3-Clause | 2,484 stars, 29 issues; historical standard for WebCodecs demux | MP4 only; Mediabunny covers more formats | EVALUATE (only if Mediabunny fails on some real MP4) | <https://github.com/gpac/mp4box.js>, npm 2026-11-14 |
| mp4-muxer | MP4 mux | 5.2.2, 2025-07-02 | 2025-09-21 | MIT | 619 stars | DEPRECATED: the README says it is replaced by Mediabunny and unmaintained | AVOID (migrate to Mediabunny) | <https://github.com/Vanilagy/mp4-muxer> (README, 2026-11-14) |
| webm-muxer | WebM mux | 5.1.4, 2025-07-02 | 2025-07-03 | MIT | 341 stars | Same author; same situation (replaced by Mediabunny; the webm-muxer README was not read, but the releases/commits stop on the same day as mp4-muxer: INFERENCE) | AVOID | npm/gh 2026-11-14 |
| web-demuxer | WebCodecs demux via FFmpeg WASM (bilibili) | 4.0.0, 2025-12-20 | 2025-12-20 | MIT (gh; npm does not declare) | 203 stars, 6 issues | Niche; overlaps Mediabunny; license of the embedded FFmpeg WASM (LGPL/GPL?) NOT verified | AVOID in the core (Mediabunny covers it) | gh/npm 2026-11-14 |
| @ffmpeg/ffmpeg (ffmpeg.wasm) | FFmpeg compiled to WASM for the browser | @ffmpeg/ffmpeg 0.12.15, 2025-04-07; core 0.12.10, 2025-04-07; GitHub release v12.15 of 2025-01-07 | 2026-09-29 (only issues/CI; last code commit seen 2025-09-17) | Wrapper MIT; `@ffmpeg/core` and `core-mt` = GPL-2.0-or-later (npm) | 17,957 stars, 459 open issues | Core GPL-2.0+ (strong copyleft if distributed together); core unpacked 64.7 MB (umd+esm; individual wasm ~32 MB: INFERENCE, not measured); multi-thread requires SAB/COOP/COEP; no new release for ~7 months; WASM32 memory limit of 2 GB (training knowledge, NOT verified this session); slow vs native | AVOID as the main engine; EVALUATE as an optional fallback for exotic formats | npm view @ffmpeg/core license (2026-11-14); gh api (2026-11-14); the doc <https://ffmpegwasm.netlify.app/docs/overview> does not give license/size/limits |

### B2. Render, preview, composition, overlay

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source |
|---|---|---|---|---|---|---|---|---|
| pixi.js | 2D renderer WebGL/WebGPU, v8 | 8.23.0, 2026-11-11 | 2026-11-11 | MIT | 48,712 stars, 422 issues; large ecosystem | Text is our own pipeline (parity with FFmpeg, see B6); WebGPU in v8 without confirmed maturity of all features (INFERENCE) | ADOPT for the preview/export compositor (evaluate direct WebGPU only if performance requires it) | gh/npm 2026-11-14 |
| Direct WebGPU | Own compositor in WGSL | n/a | n/a | n/a | Chrome 113+, Safari 26, Firefox Win/Mac | A lot of our own code (blend, transforms, effect shaders); partial Linux coverage (A2); needs the WebGL2 fallback | EVALUATE (performance spike; not for v1 if Pixi is enough) | A2 above |
| konva | Canvas 2D scene graph, transformers (selection handles) | 10.9.1, 2026-10-31 | 2026-11-10 | MIT (repo LICENSE + npm; GitHub API returns NOASSERTION because of the KineticJS header) | 14,953 stars, 3 issues | Canvas 2D (no GPU); use only for the manipulation overlay, not for the final render | ADOPT for the manipulation overlay (or implement handles in DOM/SVG) | gh/npm 2026-11-14; LICENSE (gh api) |
| fabric | Canvas 2D with interactive objects/text | 7.4.0 (npm 2026-10-31); GitHub release v740 of 2026-05-18 | 2026-11-10 | MIT | 31,557 stars, 469 issues | Heavier; more open bugs | EVALUATE (alternative to Konva) | gh/npm 2026-11-14 |
| remotion | Video as React; SSR with headless Chrome; client-side render (`@remotion/web-renderer`) | 4.0.543, 2026-11-11 | 2026-11-14 | Remotion License (not OSI). Free: individuals, for-profit with up to 3 employees, NGOs, evaluation without commercial use. Others need a Company License. Copying/modifying Remotion to sell/rent/license a derivative is forbidden. It changes "slightly" in v5 | 73,149 stars, 195 issues | NOT open source (your MIT product is incompatible if you embed or redistribute Remotion); client-side stable since 4.0.491, but the `web-renderer` page still warns "experimental"; only a CSS subset | AVOID as a dependency (incompatible with the project's MIT); use as an architecture REFERENCE | <https://raw.githubusercontent.com/remotion-dev/remotion/main/LICENSE.md>, <https://www.remotion.dev/docs/client-side-rendering/index.md> (2026-11-14) |
| @revideo/core | Programmatic video in TS (fork of Motion Canvas), headless Chrome render | 0.11.0, 2026-07-10 | 2026-07-15 | MIT | 4,210 stars, 63 issues; "engine behind Midrender" | No release for 4 months; anonymous PostHog telemetry (disable with DISABLE_TELEMETRY=true); the model is "scene code" (TS generators), not a declarative document; GitHub has no published releases (404) | EVALUATE as a reference ("same code in preview and export via headless"); AVOID as an editor base | gh, README raw (2026-11-14) |
| @motion-canvas/core | TS animation with editor | 3.17.2, 2024-12-14 | 2026-07-02 | MIT | 19,828 stars, 178 issues | No release for almost 2 years (commits still in 2026) | AVOID | gh/npm 2026-11-14 |
| @theatre/core | Keyframe animation with visual editor | 0.7.2, 2024-05-19 | 2024-08-14 | Apache-2.0 | 12,996 stars | Stalled for 2+ years | AVOID | gh/npm 2026-11-14 |
| etro | Video framework on canvas/WebGL | v0.14.1, 2026-08-12 | 2026-11-04 | GPL-3.0 | 1,156 stars | GPL-3.0 is incompatible with MIT distribution | AVOID | gh 2026-11-14 |

### B3. Animation, motion design

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source |
|---|---|---|---|---|---|---|---|---|
| gsap | JS animation/timeline | 3.15.1, 2026-11-11 | 2026-11-04 | "Standard no-charge license" (Webflow): free including commercial use, and old paid plugins (SplitText, MorphSVG). Restriction: do not use it in a visual tool that helps create a product competing with Webflow's animations without consent. Not OSI | 30,622 stars | A video editor with a visual animation UI falls into a grey zone ("no-code visual animation builder"). Not open source (MIT) | AVOID in the core (license risk); acceptable only as an internal detail, but OPINION: not worth the risk | <https://gsap.com/standard-license/> (2026-11-14) |
| motion / framer-motion | Animation for React/JS | 14.14.0, 2026-11-13 | 2026-11-13 | MIT | 35,144 stars, 11 issues | For UI, not for frame-by-frame video animation | ADOPT only for UI animation (optional) | gh/npm 2026-11-14 |
| lottie-web | Lottie player (SVG/canvas) | 5.13.0, 2025-05-21 | 2025-09-01 | MIT | 32,414 stars, 862 issues | Slow maintenance pace | EVALUATE | gh/npm 2026-11-14 |
| @lottiefiles/dotlottie-web | dotLottie player (ThorVG WASM), canvas | 0.83.2, 2026-11-12 | 2026-11-12 | MIT | 945 stars, 22 issues | Pre-1.0 (0.x) | EVALUATE (better than lottie-web for canvas rendering/frame-seek: INFERENCE) | gh/npm 2026-11-14 |
| @rive-app/canvas | Rive runtime | 2.50.0, 2026-11-09 | 2026-11-14 | MIT (runtime) | 1,022 stars (rive-wasm) | Proprietary .riv format; Rive's editor is closed | EVALUATE for animated templates | gh/npm 2026-11-14 |

### B4. Audio and waveform

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source |
|---|---|---|---|---|---|---|---|---|
| wavesurfer.js | Waveform/player | 8.1.0, 2026-11-12 | 2026-11-12 | BSD-3-Clause | 10,472 stars, 10 issues | Decodes the entire audio in the browser by default (accepts precomputed peaks) | EVALUATE (or draw our own waveform from peaks computed in the worker) | gh/npm 2026-11-14 |
| peaks.js | Waveform/segments (BBC) | npm 4.0.0 (2025-08-30); GitHub latest v3.4.2 (2024-08-15) | 2026-10-21 | LGPL-3.0 | 3,412 stars | LGPL-3.0 (care with static bundles); npm and GitHub versions diverge | AVOID | gh/npm 2026-11-14 |
| audiowaveform (BBC) | Peaks CLI (C++) | 1.10.3, 2025-08-20 | 2025-08-24 | GPL-3.0 | 2,171 stars | GPL-3.0; external binary (use as a process is acceptable, but distribution requires care) | AVOID; compute peaks with WebCodecs/FFmpeg server | gh 2026-11-14 |

### B5. UI, state, undo/redo, timeline

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source |
|---|---|---|---|---|---|---|---|---|
| react | UI | 19.3.1, 2026-11-12 | n/a | MIT | standard | - | ADOPT | npm 2026-11-14 |
| vite | Build/dev | 8.3.8, 2026-11-11 | n/a | MIT | standard | v8 is a recent major (INFERENCE; changelog not read) | ADOPT | npm 2026-11-14 |
| tailwindcss | Utility CSS | 4.3.5, 2026-11-11 | n/a | MIT | standard | - | ADOPT | npm 2026-11-14 |
| shadcn/ui | Components copied into the repo | shadcn@4.23.0, 2026-10-27 | 2026-11-13 | MIT | 128,179 stars, 1,778 issues | The code is copied (you maintain it) | ADOPT | gh 2026-11-14 |
| zustand | State | 5.1.0, 2026-11-06 | 2026-11-06 | MIT | 58,922 stars, 2 issues | - | ADOPT | gh/npm 2026-11-14 |
| immer | Immutable state + patches | 11.2.10, 2026-11-10 | 2026-11-10 | MIT | 29,002 stars, 58 issues | `produceWithPatches` gives patches and inverses for undo/redo; cost on very large documents (INFERENCE) | ADOPT | gh/npm 2026-11-14 |
| zundo | Undo/redo for Zustand | 2.3.0, 2024-11-17 | 2026-01-30 | MIT | 899 stars | Full snapshots by default; no release for 2 years | EVALUATE; OPINION: prefer our own Immer patches (inverses) | gh/npm 2026-11-14 |
| yjs | CRDT (collab/undo) | 13.6.33, 2026-09-23 | 2026-11-09 | MIT (npm and LICENSE; GitHub API returns NOASSERTION) | 23,061 stars, 161 issues | Only if real-time collaboration is a requirement; adds a mental model | EVALUATE (only if multi-user) | gh/npm/LICENSE 2026-11-14 |
| @automerge/automerge | CRDT | 3.5.0, 2026-09-16 | 2026-11-13 | MIT | 6,736 stars | Same rationale as yjs | EVALUATE (same) | gh/npm 2026-11-14 |
| fast-json-patch | JSON Patch (RFC 6902) | 3.1.1, 2022-06-17 | n/a | MIT | n/a | Unmaintained since 2022 | AVOID; Immer patches are already RFC 6902-compatible (INFERENCE to verify) | npm 2026-11-14 |
| @dnd-kit/core | Drag and drop | 6.3.1, 2024-12-05 (the repo has new packages `@dnd-kit/geometry@0.5.3`, 2026-10-31) | 2026-10-31 | MIT | 17,795 stars, 155 issues | Classic line stalled; new 0.x line in progress (migration uncertain) | EVALUATE; for the timeline, probably our own pointer events (OPINION) | gh/npm 2026-11-14 |
| @xzdarcy/react-timeline-editor | React timeline (actions/rows) | 1.0.0, 2026-01-25 | 2026-01-25 | MIT | 801 stars, 42 issues | One release; "actions" semantics are simple, not media (video/audio), no advanced waveform/zoom (INFERENCE; not tested) | EVALUATE (reference; likely write our own timeline) | gh/npm 2026-11-14 |
| @twick/* (Twick) | React video editor SDK | 0.15.31, 2026-03-18 (GitHub) / 2026-05-07 (npm) | 2026-06-04 | Sustainable Use License v1.0 (LICENSE.md; not OSI) | 548 stars, 11 issues | Source-available license | AVOID | gh, LICENSE.md (2026-11-14) |
| designcombo/react-video-editor ("OpenVideo") | Full editor + SDK over Remotion | n/a | 2026-06-30 | "OpenVideo License" (same text as Remotion: free up to 3 employees, a larger company pays; forbids a derivative for sale) | 1,836 stars, 21 issues; description "online react video editor using remotion" | Not OSI; depends on Remotion | AVOID (UI reference) | gh LICENSE (2026-11-14) |
| OpenCut | Open-source web editor ("CapCut alternative") | n/a | 2026-09-24 | MIT | 98,347 stars, 386 issues | Architecture not evaluated | EVALUATE as a reference for UX and pipeline | gh 2026-11-14 |
| OpenReel Video | React + WebCodecs + WebGPU browser editor, embeddable modular engine | n/a | 2026-10-17 | MIT | 5,664 stars, 30 issues | Code not evaluated | EVALUATE as a reference (closest to your stack) | gh 2026-11-14; description from <https://github.com/Augani/openreel-video> |

### B6. Captions, text, fonts

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source |
|---|---|---|---|---|---|---|---|---|
| subsrt-ts | SRT/VTT/ASS/other parser/converter | 2.1.3, 2026-10-28 | n/a | MIT | n/a | I did not verify ASS quality | EVALUATE | npm 2026-11-14 |
| ass-compiler | ASS parser (used by libass-wasm/JS players) | 0.1.16, 2025-11-09 | n/a | MIT | n/a | Parse only | EVALUATE | npm 2026-11-14 |
| jassub | Renders ASS/SSA via libass WASM in Worker/canvas | 2.5.37, 2026-11-08 | 2026-11-08 | MIT for the JS code (gh); the npm package declares a compound expression with LGPL-2.1-or-later, FTL/GPL-2.0+, ISC, Zlib etc. (because of the embedded libass/FreeType/HarfBuzz/fribidi) | 211 stars, 1 issue | Compound license with LGPL (static WASM is a legal review matter); same library (libass) used by the FFmpeg subtitles filter: good for ASS parity | EVALUATE (strong candidate for browser/FFmpeg parity in ASS captions) | gh + npm 2026-11-14 |
| libass-wasm | Old fork | 4.1.0, 2022-12-04 | n/a | same compound license | n/a | Abandoned | AVOID | npm 2026-11-14 |
| libass | ASS renderer (C) | 0.17.5, 2026-06-24 | 2026-11-10 | ISC | 1,194 stars | - | (reference) parity baseline with FFmpeg | gh 2026-11-14 |
| troika-three-text | SDF text in Three.js | 0.53.1, 2026-11-02 | n/a | MIT | n/a | Depends on Three.js; not on the Pixi path | AVOID | npm 2026-11-14 |
| opentype.js | Font parser/paths | 2.0.0, 2026-05-06 | 2026-08-08 | MIT | 5,047 stars, 213 issues | No full shaping (general knowledge; not verified) | EVALUATE | gh/npm 2026-11-14 |
| fontkit | Font parser with simple shaping | 3.0.0, 2026-11-11 | (repo rsms/fontkit stalled since 2019; the npm `fontkit` package comes from another repo) | MIT | n/a | Package repo not verified | EVALUATE | npm 2026-11-14 |
| harfbuzzjs | HarfBuzz WASM (shaping) | 1.6.4, 2026-10-22 | 2026-10-27 | MIT | 294 stars | Low level; you need to render the glyphs | EVALUATE (if own text with shaping identical to libass/FFmpeg is a requirement) | gh/npm 2026-11-14 |

Text parity problem: see C2.

### B7. SDKs / WebCodecs editing engines

| Name | Function | Latest release | Last commit | License | Maturity | Risks | Verdict | Source |
|---|---|---|---|---|---|---|---|---|
| @diffusionstudio/core | Browser video composition engine (timeline, text, captions, keyframes, masks) on top of WebCodecs and Mediabunny | npm 4.0.3, 2025-11-30; GitHub v1.1.1, 2024-10-29 | 2025-11-18 | npm: MPL-2.0; README: "free if you keep the 'Made with Diffusion Studio' watermark; removing it requires a paid license" | 1,250 stars, 7 issues | Contradiction between the npm license (MPL-2.0) and the README (mandatory watermark/paid license): resolve before any use; no commits for ~12 months; README recommends Remotion for SSR | AVOID as a base; EVALUATE as a reference | <https://raw.githubusercontent.com/diffusionstudio/core/main/README.md>, npm, gh (2026-11-14) |
| WebAV (@webav/av-cliper, av-canvas) | WebCodecs SDK (decode, composition, export), focused on Chrome/Edge/Electron | 1.2.8, 2026-01-10 | 2026-01-10 | MIT (gh; npm does not declare) | 2,101 stars, 5 issues | No commits for 10 months; non-English as the main language (INFERENCE) | EVALUATE as a reference | gh/npm 2026-11-14 |
| IMG.LY CE.SDK, combo SDK | Commercial SDKs | n/a | n/a | Commercial | n/a | Not open source | AVOID | <https://img.ly/products/video-for-web/> (search result) |
| combo-sdk-js (designcombo) | n/a | - | - | - | - | `gh api repos/designcombo/combo-sdk-js` returned 404 (repo does not exist or is private) | NOT VERIFIED | gh 2026-11-14 |

### B8. License notes affecting the project's MIT license

- Incompatible or risky: Remotion (not OSI), OpenVideo/designcombo (same license), Twick (SUL), GSAP (Webflow clause), etro (GPL-3.0), peaks.js (LGPL-3.0), audiowaveform (GPL-3.0), @ffmpeg/core (GPL-2.0+), Diffusion Studio (watermark).
- Needs attention: Mediabunny (MPL-2.0: per-file copyleft; OK in an MIT product), jassub (LGPL/GPL chain in the WASM).
- Clean (MIT/BSD/Apache/ISC): pixi, konva, fabric, react, vite, tailwind, zustand, immer, motion, dotlottie, rive, wavesurfer (BSD-3), mp4box (BSD-3), libass (ISC).

---

## PART C: Questions with evidence

### C1. Client-side WebCodecs render in 2026: 1080x1920, up to 5 min, acceptable quality?

Short answer (INFERENCE + FACTS): technically yes in desktop Chrome, with operational caveats; I did not find a public benchmark for 1080x1920 x 5 min.

- Who does it in production:
  - Remotion `@remotion/web-renderer`: stable since 4.0.491, encodes with Mediabunny/WebCodecs, no FFmpeg (<https://www.remotion.dev/docs/client-side-rendering/index.md>). FACT. The page does not cite production users nor resolution/duration limits.
  - Descript: decode, composition and encode on the GPU via WebCodecs, "2 to 3x faster 4K exports on average" and "memory: 33 MB per decoded 4K frame" (<https://www.descript.com/blog/article/the-new-descript-how-we-multiplied-the-apps-speed-and-performance>). Vendor FACT (Descript is Electron, Chromium engine).
  - IMG.LY CE.SDK: client-side export via WebCodecs in the browser; engine up to 4 GB (desktop) / 2 GB (mobile); it notes that open-source Linux builds without H.264 need server-side export (<https://img.ly/docs/cesdk/js/create-video/limitations-6a740d/>). Vendor FACT.
  - OpenReel, OpenCut, WebAV, Diffusion Studio: open-source projects with the same approach (B7); performance and stability not measured here.
- Reported problems: (1) GPU leak from VideoFrame without `close()`; (2) unbounded prefetch becomes a memory problem on long video; (3) decoding the entire audio is a hidden cost; (4) the buffer of the final MP4 in the muxer dominates memory (anecdotal report in a Hashnode comment); (5) Firefox: 2 min exports where Chrome takes ~10 s, or crashes (secondary source Kinetix: <https://simplearyan.github.io/kinetix/blog/browser-export-performance>); (6) rAF throttling in a background tab (Remotion limitations, FACT); (7) Linux without an H.264 encoder (CE.SDK, FACT) and without HW acceleration (A1). Secondary sources: <https://hackernoon.com/how-to-optimize-video-export-pipelines-in-browser-based-editors>, <https://dev.to/nareshipme/why-we-switched-to-streaming-frame-extraction-for-mobile-video-editing-3cea>.
- Order-of-magnitude calculation (my INFERENCE): 1080x1920 = 2.07 Mpx; RGBA 8-bit ≈ 8.3 MB per frame (vs 33 MB at 4K, FACT Descript); 5 min at 30 fps = 9,000 frames. With HW decode/encode, memory depends on the window of frames in flight (not on the duration) if there is backpressure and streaming mux to disk; quality is that of the encoder (HW H.264 tends to be less efficient than x264 at the same bitrate, general knowledge NOT verified: use a generous bitrate or the equivalent CRF on the server).
- Recommendation (OPINION): client-side as the fast path/"preview export" in desktop Chrome with a positive `isConfigSupported`; keep server export (FFmpeg or headless Chrome) as the guaranteed path for AI agents, automation, Linux and weak devices. Experiment needed: measure 1080x1920 x 5 min on 3 machines (HW H.264 vs software).

### C2. Cost of TWO renderers (browser canvas + FFmpeg on the server)

Evidence:

- Remotion itself documents divergence within its own ecosystem: SSR is a "pixel-perfect screenshot" of Chrome, while client-side "rebuilds layout" and supports less CSS; text can differ in pixels, glyphs/SVG text differ between engines, font-stretch is rounded, filters differ (<https://www.remotion.dev/docs/client-side-rendering/limitations>, <https://www.remotion.dev/docs/client-side-rendering/how-it-works>). FACT. If Remotion, with the same React code, suffers this across two render paths, FFmpeg `drawtext`/libass vs 2D canvas will have more differences (INFERENCE).
- Color/gamma: Remotion renders in sRGB, headless Chrome does not support HDR and converts HDR to SDR in OffthreadVideo via FFmpeg/zscale (<https://www.remotion.dev/docs/hdr>). FACT. The typical divergence in FFmpeg pipelines comes from color metadata (primaries/transfer/matrix) at encode (reports from Blender/VDMX; secondary sources). INFERENCE: set explicit BT.709 limited tags on both paths and test with golden frames.
- Text: libass (used by the FFmpeg `subtitles`/`ass` filters) is the reference for ASS; jassub embeds the same libass in WASM, so ASS has near-guaranteed parity; FFmpeg `drawtext` (FreeType, no advanced shaping by default) does not match the browser's 2D canvas/HarfBuzz (INFERENCE; not tested). Options: (a) text always rendered to bitmap/vector by the SAME code and sent to FFmpeg as an overlay; (b) captions via ASS on both sides (jassub + libass); (c) HarfBuzz WASM on both sides.
- Real-world mitigation: Remotion and Revideo export with the same composition code in headless Chrome (screenshots/frames) and then encode via FFmpeg (Remotion) or render in a headless browser (Revideo: "anywhere Node and a headless browser run", <https://raw.githubusercontent.com/redotvideo/revideo/main/README.md>). FACT. Cost: one browser per render worker, heavy concurrency (one Chrome per job), but pixel parity with the preview.
- Hybrid option (OPINION, preferred): project document (JSON) + a single TS "composition renderer" (Pixi/WebGPU) that runs in the browser (preview, client-side export) AND in headless Chrome on the server (WebCodecs/Mediabunny or frame capture to FFmpeg only for encode/mux/audio). FFmpeg stays only for decode/encode/mux/audio, never for composing text/effects. This eliminates the "second renderer".
- Parity test: per-frame golden images of a reference project running on both paths with a tolerance (PSNR/SSIM), in CI.

### C3. Smooth scrubbing: what the projects use

Evidence and synthesis:

- `<video>` + seek: simple, uses the browser's decoder, but has no frame-accurate stepping and no multi-layer sync (Descript, vendor FACT; seek precision: GAP in a primary source, A10).
- WebCodecs with frame cache: Descript (GPU, zero-copy, on-demand chunks); Mediabunny offers `getSample(t)` (last frame <= t), `samplesAtTimestamps` (batch, avoids decoding the same packet twice), `CanvasSink` with `poolSize` for constant VRAM; there is no general cache for VideoSampleSink (<https://mediabunny.dev/guide/media-sinks>). FACT. You need to build: an LRU cache of recent frames, decoding from the previous keyframe up to the target (discarding intermediate frames), and cancellation of requests during fast drag.
- Low-resolution proxies: Descript transcodes media on the server to a consistent format and streams small chunks adjusted to system capacity and window size (same Descript URL). FACT. It is the standard approach of professional editors (general knowledge, OPINION).
- Thumbnail sprites/strips for the timeline track: I did not find a primary source in this session; common use (`samplesAtTimestamps` can generate them) (INFERENCE).
- Recommendation (OPINION): three layers: (1) thumbnails/sprites in OPFS for coarse scrubbing on the timeline; (2) an intra-frame/short-GOP proxy (e.g., H.264 540p with low keyint) generated by FFmpeg or WebCodecs for fast stepping; (3) WebCodecs decode with an LRU cache for a stopped position/precise playback; `<video>` only as a simple fallback. Measure decoding of 1080x1920 H.264 with long GOP (phone videos) to decide whether a proxy is mandatory.

---

## Gaps (what was not verified)

1. Do WKWebView and WebKitGTK actually expose WebCodecs/WebGPU/OPFS? No source; requires a spike on a real machine (A15).
2. Memory limits per tab/ArrayBuffer/WASM in stable Chrome, and the memory64 status in Safari.
3. HW/SW codec matrix per OS (H.264/HEVC/AV1/AAC encode) and official Chrome behavior on Linux (A1).
4. `<video>` seek precision (keyframe vs exact) without a primary source (A10).
5. Current support for AudioWorklet and WebTransport, and VFR behavior (A9, A13, A14).
6. License of the WASM embedded in web-demuxer; exact size of the ffmpeg.wasm files; the 2 GB limit (B1); webm-muxer README not read; combo-sdk-js does not exist (404).
7. Real benchmarks of 1080x1920 x 5 min client-side export (C1) and of text/color parity between canvas and FFmpeg (C2): both need experiments.
8. Release dates for packages without GitHub Releases are npm `time.modified` (approximation). Issue counts include PRs.
