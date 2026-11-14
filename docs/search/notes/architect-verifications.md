# Verifications personally performed by the architect

Critical facts brought by the subagents and re-checked against the primary source by the architect. Access date for all rows: 2026-11-14. Method: `gh api` / `gh release list` against GitHub, or direct reading of the cited file.

| # | Claim | Result | Source |
| --- | --- | --- | --- |
| 1 | `opencut-app/opencut` is today `OpenCut-app/OpenCut`, MIT, 98,347 stars, not archived, last push on 2026-09-24 | CONFIRMED | `gh api repos/OpenCut-app/OpenCut` |
| 2 | OpenCut's actual editor is in `OpenCut-app/opencut-classic`, archived, last push on 2026-05-17 | CONFIRMED (`archived=true`, 298 stars, MIT) | `gh api repos/OpenCut-app/opencut-classic` |
| 3 | OpenCut's latest release is `v0.3.0` (2026-04-15); the newest entry in the list is a pre-release build of FFmpeg (`ffmpeg-8.1.3-1`, 2026-09-24) | CONFIRMED | `gh release list -R OpenCut-app/OpenCut` |
| 4 | Wails v3 is stable: `v3.1.0` marked as Latest on 2026-11-01; `v2.17.1` came out the same day; `v3.2.0` nightlies in progress | CONFIRMED | `gh release list -R wailsapp/wails` |
| 5 | Tauri v3 is in alpha (`tauri v3.0.0-alpha.6`, 2026-11-06) and the package `tauri-runtime-cef v3.0.0-alpha.7` exists (embedded Chromium); the stable line is v2 (`tauri-cli v2.12.4`, 2026-10-21) | CONFIRMED | `gh release list -R tauri-apps/tauri` |
| 6 | Electron stable is `v45.3.2` (2026-11-13) | CONFIRMED | `gh release list -R electron/electron` |
| 7 | The official MCP Go SDK (`modelcontextprotocol/go-sdk`) is at `v1.8.0` (2026-09-14), 5,346 stars, last push on 2026-11-13 | CONFIRMED | `gh release list`, `gh api` |
| 8 | MCP Go SDK license: GitHub returns `NOASSERTION` because the project is transitioning from MIT to Apache-2.0 (new contributions under Apache-2.0; older ones without consent remain MIT). Both are compatible with an MIT product | CONFIRMED (text of the `LICENSE` file) | `gh api repos/modelcontextprotocol/go-sdk/contents/LICENSE` |
| 9 | The `2026-07-28` revision of the MCP specification exists (folders in `docs/specification`: 2024-11-05, 2025-03-26, 2025-06-18, 2025-11-25, 2026-07-28, draft) | CONFIRMED that the revision exists. The content ("stateless, no `initialize`, no `Mcp-Session-Id`, tasks as an extension") has NOT YET been re-checked by the architect | `gh api repos/modelcontextprotocol/modelcontextprotocol/contents/docs/specification` |
| 10 | vukem's default queue uses the `jobs` table (`QUEUE_TABLE` default `jobs`) | CONFIRMED | `vukem/background.go:78` |

| 11 | OpenCut classic's `MediaTime` is an `i64` of ticks with `TICKS_PER_SECOND = 120_000` | CONFIRMED in the clone (`rust/crates/time/src/media_time.rs`, lines 10 and 16) | file reading |
| 12 | The 13-issue sample cited in `notes/engine-postmortems.md` exists and covers the assigned topic: remotion #12603 (preview 1 frame behind the render), #11315 (text with fallback font in the web renderer), #5509 (iPhone video rotation); mediabunny #144 (`VideoFrame` leak), #447 (edit list), #69 (rational time in the package); ffmpeg.wasm #876 (4 GB support, open), #451 (OOM, open); OpenCut #914 (export stuck at 5%, open); editly #49 (duration changes with FPS, open); olive #1705 (undo does not undo the audio); obs-studio #13396 (VFR in remux); w3c/webcodecs #626 (audio priming/padding, open) | CONFIRMED (13 of 13: existence, title and state) | `gh api repos/<o>/<r>/issues/<n>` |

| 13 | The MCP `2026-07-28` revision is stateless: it removes sessions and the `Mcp-Session-Id` header, removes the `initialize` handshake, moves tasks to the extension `io.modelcontextprotocol/tasks` (polling via `tasks/get`), and marks Roots, Sampling, Logging and OAuth dynamic client registration as deprecated | CONFIRMED in the specification changelog | `gh api repos/modelcontextprotocol/modelcontextprotocol/contents/docs/specification/2026-07-28/changelog.mdx` |
| 14 | Competitors and agent-first references exist and are active: `storytold/filmcraft` (18,267 stars, Apache-2.0 per the API, created 2026-09-30, push on 2026-11-14), `diffusionstudio/editor` (3,510 stars, MPL-2.0, push on 2026-10-18), `walterlow/freecut` (2,329 stars, MIT, push on 2026-10-29), `omni-media/omnitool` (44 stars, MIT, push on 2026-11-02), `heygen-com/hyperframes` (84,236 stars, Apache-2.0, push on 2026-11-14) | CONFIRMED | `gh api repos/<o>/<r>` |

| 13 | MCP revision `2026-07-28` is stateless: it removes protocol sessions and the `Mcp-Session-Id` header, removes the `initialize` handshake, moves tasks into the extension `io.modelcontextprotocol/tasks` (polling with `tasks/get`), and deprecates Roots, Sampling, Logging and OAuth dynamic client registration | CONFIRMED in the specification changelog | `gh api repos/modelcontextprotocol/modelcontextprotocol/contents/docs/specification/2026-07-28/changelog.mdx` |
| 14 | Agent-first competitors and references exist and are active: `storytold/filmcraft` (18,267 stars, Apache-2.0 according to the API, created 2026-09-30, pushed 2026-11-14), `diffusionstudio/editor` (3,510 stars, MPL-2.0, pushed 2026-10-18), `walterlow/freecut` (2,329 stars, MIT, pushed 2026-10-29), `omni-media/omnitool` (44 stars, MIT, pushed 2026-11-02), `heygen-com/hyperframes` (84,236 stars, Apache-2.0, pushed 2026-11-14) | CONFIRMED | `gh api repos/<o>/<r>` |
| 15 | `go-text/typesetting` is at tag `v0.3.5` (commit of 2026-09-04), licensed `Unlicense OR BSD-3-Clause`, described by its README as pure Go and shared by the Fyne, Gio and Ebitengine toolkits; last push 2026-11-10 | CONFIRMED | `gh api repos/go-text/typesetting` (tags, LICENSE, README) |
| 16 | vukem `v0.43.0` facts used by the architecture: commands are registered through the `vukem.Command` interface with the standard `flag` package and exit codes through `ExitError` (`command.go:45`, `command.go:1082`); the core has an SSE hub (`sse/hub.go`); `contrib/engine` exists at tag `contrib/engine/v0.1.1` and `drivers/websocket` at `v0.1.6`; the queue has no per-job progress or cancel API; there is no `plans/` folder | CONFIRMED in the local clone of vukem | files cited |

## Pending personal verification

- Official "CapCut x Codex" integration via MCP and Canva's official MCP (`projects/capcut.md` §8, `projects/canva.md`).
- WebCodecs / WebGPU / OPFS support in WKWebView and WebKitGTK: cross-check with `notes/stack-browser.md`.
- Items marked `[training]` and "title only" in `notes/engine-postmortems.md`: they cannot become a decision basis without checking.
- Official loudness targets (LUFS) of the platforms: no primary source so far.
