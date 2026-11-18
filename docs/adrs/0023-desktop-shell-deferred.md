# ADR-0023: Desktop: the same binary on localhost now; a packaged shell in BETA, Wails v3 first candidate

- Status: Accepted (2026-11-14); the shell is Wails v3 (author decision of 2026-11-15, item 5), confirmed as the only desktop by [ADR-0041](0041-one-web-ui-and-the-wails-shell.md) (2026-11-18); the detection runs on macOS and Linux are still owed
- Deciders: author (D-004), architect
- Links: author decision D-004, stack trade-offs §7, ADR-0003, ADR-0007, ADR-0013

## Context

The briefing wants web and desktop, with "Go + possibly Rust + webview" and a native renderer as an option. D-004: web first; the first "desktop" is the Go binary serving the UI on localhost; a packaged app comes in BETA after a test on real machines. Wails v3 became stable on 2026-10-26, needs Go 1.26 or later and has a server mode; it uses the system webview. WebView2 on Windows is Chromium; for WKWebView and WebKitGTK no evidence of usable WebCodecs or WebGPU was found. Tauri v3 with a Chromium runtime is in alpha. Electron bundles Chromium.

## Decision

1. Through the Prototype and ALPHA, desktop use is `vukem-video serve`, which opens the default browser on the local address (Chrome is the tested one, PRD-001 §7). `desktop/` holds only a feature-detection test page and its results.
2. The packaged application in BETA is a **shell around the same binary and the same UI**: it starts the Go process, opens a window on its local address, and adds native menus, file dialogs, auto-update and code signing. It contains no editing logic and no renderer.
3. No Rust, no native renderer, no gRPC, no second UI.
4. The first candidate is **Wails v3**, because it is Go and its server mode is this architecture. The preview needs only `<video>` playing VP9 in MP4 with AAC (the proxy of SDD-011 §4.2), a WebGL2 canvas, `EventSource` and Pointer Events (ADR-0013), which is the floor the system webviews must meet.
5. The choice is confirmed only by a test: the detection page run inside WKWebView on macOS and WebKitGTK 6.0 on Linux, checking that floor and measuring proxy playback. If either fails, the shell is **Electron** on the failing platforms or everywhere.

   **Author decision (2026-11-15): the shell is Wails v3, at exactly `github.com/wailsapp/wails/v3` v3.1.0, for performance.** The dependency is approved. The shell is the Go module `desktop/shell/app` (so the server binary and the root `go.mod` do not pull it); the code that needs no window (the child process and the navigation policy) is the package `desktop/shell`. The detection runs of P3-DSK-001 still confirm the webview floor; a `fail` verdict reopens this item for that platform. Wails v3.1.0 has no hook that vetoes a navigation before it happens, so the shell sends the window back to the local address after any navigation that ends elsewhere (`shell.GuardScript`); a native veto needs a Wails change.
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

## Results of the detection runs

The runs of the detection page (`desktop/detect`, SDD-012 section 21.2) are versioned one file per run in `desktop/results/`, with a note per day in `desktop/results/verdict.md`. This note records data only; the status of the shell choice stays with the author.

| Engine | Platform | Runs | Verdict | `video-seek` p50 and p95 | NFR-003 (100 ms) |
| --- | --- | --- | --- | --- | --- |
| Chromium 156 (Playwright build) | Windows 11 | 2026-11-15 headless, 2026-11-16 headed | pass | 3.7 ms and 6 ms; 10 ms and 21 ms | met |
| Edge 157, the engine of WebView2 | Windows 11 | 2026-11-15 headless, 2026-11-16 headed | pass | 7.1 ms and 10.9 ms; 7 ms and 26 ms | met |
| WKWebView | macOS | not run | n/a | n/a | n/a |
| WebKitGTK 6.0 | Linux | not run | n/a | n/a | n/a |

The first headed Edge run of 2026-11-16 has verdict `fail` on the `pointer` test only: the scripted drag sent too few moves for the page to count them. The second, with `steps` on every move, passes; both files are kept.
