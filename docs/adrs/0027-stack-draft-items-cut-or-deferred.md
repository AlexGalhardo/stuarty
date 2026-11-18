# ADR-0027: Items of the stack draft that were cut or deferred

- Status: Accepted (2026-11-14); amended on 2026-11-18: the Rust renderer was adopted by [ADR-0029](0029-one-renderer-webgpu-webcodecs-rust.md), HyperFrames is used as an external engine ([ADR-0042](0042-editor-scope-motion-audio-export.md)), and the triggers of "upload over the network" and of outbound features are met by [ADR-0032](0032-controlled-outbound-and-human-approval.md); the other verdicts stand
- Deciders: architect, on the author's decisions D-001 to D-016
- Links: stack trade-offs §2, §8, licensing, ADR-0001 to ADR-0026

## Context

The author's stack draft was explicitly a list of unverified candidates, with the instruction to validate, replace or cut each one and to say so when something was oversized for a minimal editor. Items that were adopted have their own ADR. This record lists every item that was not, so that nobody reintroduces one without revisiting the reason.

## Decision

**Cut** means not used and not planned. **Deferred** means not in the road to 1.0 unless the stated trigger happens.

| Item | Verdict | Reason | Trigger to revisit |
| --- | --- | --- | --- |
| gonertia | Cut | vukem has its own Inertia adapter | none |
| Immer | Cut | undo lives in the core (ADR-0006, ADR-0007) | none |
| dnd-kit | Cut | Pointer Events suffice; the classic line is unreleased since 2024-12 | none |
| WebCodecs preview, mediabunny, mp4box.js | Deferred | export is on the server and the paused frame is exact (ADR-0003) | measured scrubbing below the budget of the PRD |
| wavesurfer.js | Cut | peaks are computed at ingest and drawn directly | none |
| Web Workers for media, OPFS | Deferred | media lives on the server's disk (ADR-0010) | a browser-only or offline mode |
| WebSocket | Cut | server-sent events cover the protocol (ADR-0017) | a need for client-to-server streaming |
| PostgreSQL, pgx | Deferred | local and self-host first (D-003) | a hosted multi-user service |
| sqlc, goose | Cut | vukem's ORM and migrations | none |
| river, asynq | Cut | vukem's queue with the `database` driver (ADR-0018) | none |
| tusd | Deferred | import is by local path (ADR-0010); vukem has `upload.Tus` | upload over the network |
| S3-compatible storage | Deferred | disk first (D-003); vukem has an S3 module | a hosted service |
| cobra | Cut | vukem's command registry (ADR-0016) | vukem's runner proving insufficient |
| Protobuf, buf, gRPC | Cut | JSON Schema serves every consumer (ADR-0014); the shell talks HTTP (ADR-0023) | none |
| Vitest as a general requirement | Reduced | the browser holds little logic; used only where TypeScript has logic | none |
| Railway | Cut | only the static documentation site is hosted (D-015) | none |
| Sentry | Cut | no telemetry (D-016) | the author reverses D-016 |
| Electron | Deferred | fallback shell only (ADR-0023) | system webviews failing the BETA test |
| Rust renderer (wgpu), ffmpeg-next, GStreamer, cpal, symphonia, rubato, cosmic-text, glyphon | Cut | a third renderer for no requirement (ADR-0003, ADR-0023) | none |
| cargo-dist | Cut | no Rust | none |
| PixiJS "optional" | Adopted | see ADR-0013 | n/a |
| Tauri | Cut | Rust for a shell with the same webview limits | none |
| Remotion | Cut | source-available licence with paid tiers | none |
| Revideo, Motion Canvas | Cut | headless-browser renderers; stale releases | none |
| ffmpeg.wasm | Cut | GPL core, memory ceiling | none |
| MLT Framework, GStreamer Editing Services | Cut | would replace our plan with their model; native dependencies | none |
| OpenTimelineIO | Deferred | export and import only, in BETA | n/a |
| WebGPU directly | Cut | PixiJS abstracts it if ever needed | none |
| GSAP | Cut | licence forbids competing visual animation tools | none |
| whisper.cpp through cgo bindings | Cut | separate process instead (ADR-0021) | none |

## Alternatives

Keeping items "just in case" was rejected: each unused dependency is a version to track, a licence to check and a concept for seven agents to keep in mind.

## Consequences

Positive: the dependency list a new contributor must understand is short: vukem, the MCP Go SDK, a JSON Schema validator, a text shaping library, FFmpeg and whisper.cpp as processes, and React, Vite, Tailwind, shadcn/ui, Zustand and PixiJS in the browser.

Negative: several deferred items (PostgreSQL, S3, upload, WebCodecs) will need real work when their trigger arrives; the interfaces in `service/` and `PreviewExecutor` are where they will attach.
