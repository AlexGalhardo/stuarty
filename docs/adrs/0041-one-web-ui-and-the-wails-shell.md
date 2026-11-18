# ADR-0041: One web UI; the desktop is the Wails shell; the Gio window is removed

- Status: Accepted (2026-11-18)
- Deciders: author
- Supersedes: ADR-0030
- Confirms: ADR-0023 (the desktop is a shell around the same binary and the same UI)
- Links: author decisions D-055, D-074; ADR-0007, ADR-0013, ADR-0023, ADR-0029; SDD-012 section 21

## Context

Three user interfaces exist: the web UI in React (the complete one), the Wails shell that shows it in a native window (ADR-0023), and a native Gio window added in release 0.29.0 (ADR-0030), which lacks audio, proxy playback and most of the inspector. Stuarty adds a dashboard, an image editor, publishing and writing; each screen written twice would double the work.

The research compared the shells on 2026-11-18 (research note `web/07-stack-desktop-web-self-hosted`): Wails v3 is stable and is Go; Electron bundles Chromium; Tauri 3 with an embedded Chromium is still an alpha. The weak point of every system-webview shell is Linux: no evidence was found of WebGPU in WebKitGTK. The detection page of this repository was run only on Windows.

## Decision

1. **There is one UI: the web UI in `web/resources/`.** Every page is written once and appears in the browser and in the desktop application.
2. **The desktop application is the Wails shell** (`desktop/shell`, `desktop/shell/app`), with its packaging, its signed update feed and the detection page (`desktop/detect`).
3. **The Gio window is removed.** `desktop/gio` and what refers only to it leave the tree in their own epic.
4. **Windows and the web version in Docker are validated first.** The desktop on macOS and Linux is its own epic, when real machines are available; it starts by running the detection page in WKWebView and WebKitGTK.
5. **The shell choice is revisited only on evidence:** a `fail` verdict of the detection page on a platform reopens the question for that platform, with Electron as the fallback of ADR-0023.

## Alternatives

- **Move to Electron now.** The same Chromium everywhere, a larger application and a JavaScript main process; not needed before the detection runs say so.
- **Continue the Gio window.** Rejected: every new screen written twice.
- **Tauri 3 with embedded Chromium.** An alpha with an open GPU bug on Linux, and Rust outside the renderer.

## Consequences

Positive: one UI to build, test and translate.

Negative: until the detection runs on macOS and Linux, the desktop on those systems has no evidence.

Negative: the native look that ADR-0030 wanted is given up.
