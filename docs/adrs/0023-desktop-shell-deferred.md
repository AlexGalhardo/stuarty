# ADR-0023: Desktop: the same binary on localhost now; a packaged shell in BETA, Wails v3 first candidate

- Status: Accepted (2026-11-14); shell choice Proposed until the BETA test
- Deciders: author (D-004), architect
- Links: [brainstorming D-004](../brainstorming.md), [stack trade-offs §7](../search/stack-tradeoffs.md), ADR-0003, ADR-0007, ADR-0013

## Context

The briefing wants web and desktop, with "Go + possibly Rust + webview" and a native renderer as an option. D-004: web first; the first "desktop" is the Go binary serving the UI on localhost; a packaged app comes in BETA after a test on real machines. Wails v3 became stable on 2026-10-26, needs Go 1.26 or later and has a server mode; it uses the system webview. WebView2 on Windows is Chromium; for WKWebView and WebKitGTK no evidence of usable WebCodecs or WebGPU was found. Tauri v3 with a Chromium runtime is in alpha. Electron bundles Chromium.

## Decision

1. Through the Prototype and ALPHA, desktop use is `vukem-video serve`, which opens the default browser on the local address (Chrome is the tested one, PRD-001 §7). `desktop/` holds only a feature-detection test page and its results.
2. The packaged application in BETA is a **shell around the same binary and the same UI**: it starts the Go process, opens a window on its local address, and adds native menus, file dialogs, auto-update and code signing. It contains no editing logic and no renderer.
3. No Rust, no native renderer, no gRPC, no second UI.
4. The first candidate is **Wails v3**, because it is Go and its server mode is this architecture. The preview needs only `<video>` playing VP9 in MP4 with AAC (the proxy of SDD-011 §4.2), a WebGL2 canvas, `EventSource` and Pointer Events (ADR-0013), which is the floor the system webviews must meet.
5. The choice is confirmed only by a test: the detection page run inside WKWebView on macOS and WebKitGTK 6.0 on Linux, checking that floor and measuring proxy playback. If either fails, the shell is **Electron** on the failing platforms or everywhere.
6. Signing, notarization and auto-update are part of the BETA work, with their costs decided by the author then.

## Alternatives

- **Packaged desktop from the Prototype.** Forces webview, signing and update work before the editor exists.
- **Electron from the start.** Chromium guaranteed, about 100 MB and a Node runtime; unnecessary if the floor is met.
- **Tauri.** Adds Rust for a shell with the same webview limits.
- **Native renderer in Rust (wgpu).** A third renderer; nothing requires it.

## Consequences

Positive: web and desktop share everything but the shell; no desktop-only bugs in editing or rendering.

Positive: nothing is spent on desktop packaging until the product works.

Negative: until BETA there is no installer, icon or auto-update; users run a binary and a browser.

Negative: if the system webviews fail the floor, the fallback adds Electron's size and a JavaScript main process.
