# ADR-0049: A native capture helper per operating system

- Status: Accepted (2026-11-23, author decision D-133)
- Deciders: author
- Amends: `.agents/rules/rust.md` item 1 and AGENTS.md rule 5 (Rust lives only in `renderer/`); PRD-001 section 7 (recording is out of scope), changed by epic 61
- Links: ADR-0002, ADR-0003, ADR-0011, ADR-0029, ADR-0041, SDD-007 section 6, SDD-023 (to be written by epic 61)

This record decides how the desktop application captures the screen, sound, webcam and pointer.

## 1. Context

The product has no capture today. Global cursor and click data, which automatic zoom needs, are not available to a page in a WebView. The server is a sidecar built without cgo. A native child process already exists: `stuarty-render`, fed through pipes and started under the FFmpeg process policy (SDD-007 section 6.10).

Capture APIs differ by system: Windows Graphics Capture and WASAPI, ScreenCaptureKit, and the ScreenCast portal with PipeWire.

## 2. Decision

1. Capture runs in a separate executable, `stuarty-capture`, one build per system, started by the server through `engine/capturehost` under the process policy of SDD-007 section 6.
2. The helper is written in **Rust, in a new top-level folder `capture/`**. This is the second and only other place Rust may live. Swift is allowed inside the macOS backend where the API requires it.
3. The helper captures and writes. It decides nothing about a project. Rules stay in `core/recording`.
4. Control messages are versioned JSON on stdin and stdout. Frames reach FFmpeg through a pipe, and audio and events go to their own files on one shared clock.
5. Output is fragmented MP4, so an interrupted recording stays decodable.
6. The server remains free of cgo. Rust rules 2 to 4, 7 and 8 of `.agents/rules/rust.md` apply to `capture/`. Rules about bit-exact pixels and WebAssembly do not, because the helper composes nothing.
7. Approved crates (D-133), at exact versions, with licences read from their repositories: `windows-capture` (MIT), `wasapi` (MIT), `cpal` (Apache-2.0). `scap` (MIT) is approved only if the spike of epic 61 proves it necessary. The main Cap code (AGPLv3) is excluded.

## 3. Alternatives

| Option | Why not |
| --- | --- |
| `getDisplayMedia` in the WebView | Not validated in the three WebViews, and no global cursor or clicks. Kept for the web (epic 63) |
| FFmpeg devices (`gdigrab`, `avfoundation`, `x11grab`) | No Wayland path, weak audio and event support, and the argument allowlist of ADR-0011 would have to widen |
| cgo in the Wails shell | Breaks the no-cgo rule and complicates three builds |
| Pure Go with system calls | Feasible on part of Windows, weak on macOS and PipeWire |
| One helper in C++ or per-system languages | Larger unsafe surface and more toolchains; Rust is already pinned and reviewed here |

## 4. Consequences

- A new executable to build, ship, list in the notices and keep in the release assets.
- An unsigned helper that captures the screen may alarm antivirus software [to validate in the spike of epic 61].
- macOS permission attribution depends on signing, which is unsigned today (author decision D-033). **Epic 67 stops there until the author decides.**
- On Wayland, global click data has no generic solution. Epic 68 states the limit and promises no parity.

## 5. Decisions of the author (D-133)

1. Rust in `capture/` is accepted.
2. The crates of section 2 item 7 are approved.
3. **Still open:** an Apple Developer account for signing and notarization costs money and stays with the author. Epic 67 stops there.
