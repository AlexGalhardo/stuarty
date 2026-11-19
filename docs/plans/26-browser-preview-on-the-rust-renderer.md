# 26: The browser preview on the Rust renderer

## Goal

What a creator sees while editing is what the exported file will contain, because the same renderer draws both. The surprises between preview and export end.

## Context and related decisions

- ADR-0029 (one Rust renderer for preview and export), ADR-0003, ADR-0013 (the browser preview stack), ADR-0038.
- Author decision D-073 (finishing the Rust renderer is carried over from the earlier plans). PRD-001 NFR-011, NFR-001 to NFR-004.
- SDD-013 sections 4.7 (the browser worker protocol), 5.3 (frames through WebCodecs) and 6 (the proxy chunk index); SDD-012; SDD-004.
- Read `.agents/rules/rust.md` and `.agents/skills/renderer-parity/` before touching anything.
- Open product issues this epic closes (their text comes in the briefing): VV-093 (the preview shows black behind a chroma-keyed subject while the export does not; check it again on the current build first), VV-222 (an unhandled WebGPU exception when a text item is selected in the desktop preview).

## Scope

- Modules: `web/resources/js/editor/` (the preview executor and its worker), `renderer/` (only what the browser host needs: exports, the WebAssembly build), `scripts/build-renderer.sh`, `engine/renderhost/` (only if the plan format must change for both hosts), `web/e2e/` (parity specs), SDD-013, SDD-012.
- The preview of the video editor in Chromium-based browsers and in the Windows shell's webview first; the other engines keep the current executor as a fallback, chosen by feature detection.

## Out of scope

- Removing the PixiJS executor and the Go compositor (epic 47, after the three-system proof).
- New visual features: this epic changes who draws, not what is drawn.
- The image editor's preview (epic 28 builds on this executor when it is merged).

## Dependencies

03.

## Technical design

- **One plan**: the browser asks the server for the frame plan (the same `core/plan` output the export uses) for the visible window and draws it; the UI decides nothing about pixels (rule 2 of `AGENTS.md`).
- **Worker**: the renderer's WebAssembly module runs in a dedicated worker with the protocol of SDD-013 section 4.7; frames reach it as `VideoFrame` objects decoded from the proxy by WebCodecs through the chunk index (sections 5.3 and 6); the result is presented on an `OffscreenCanvas`. Audio stays on the existing path and remains the clock.
- **GPU first, CPU as truth**: the `wgpu` path draws playback; the CPU path is the reference the parity tests compare against, and the fallback when the GPU path is unavailable. Tolerances are the ones of ADR-0029, not new ones.
- **Feature detection**: WebCodecs, `OffscreenCanvas` and WebGPU or WebGL2 are probed once; when any is missing the current executor is used and a diagnostic line says why. A setting forces either executor, for comparison and for bug reports.
- **Parity harness**: for each golden project, the browser renders chosen frames with the new executor and the test compares them with the export's frames of the same plan, within tolerance; a failure writes both images and the difference.
- **Budgets**: frame time and dropped frames during playback of the reference project are measured by the existing performance report; the numbers are recorded, and compared with the current executor on the same machine, idle.
- **Build**: the `.wasm` is built reproducibly and pinned by digest as today.

## Tasks

- [x] 1. Read SDD-013 and the parity skill; list in SDD-013 what of sections 4.7, 5.3 and 6 is implemented today and what this epic adds. Verify: `task lint:md` exits 0.
- [x] 2. Build the worker and the protocol with a still-frame path (no playback). Verify: `task test:web` passes with the worker's unit tests and `bash scripts/build-renderer.sh` reproduces the pinned digest or updates it with the reason.
- [x] 3. Decode proxy frames through WebCodecs and the chunk index, with seeking. Verify: `web/e2e/specs/preview-rust-seek.spec.ts` (new) passes in Chromium.
- [x] 4. Present on `OffscreenCanvas` with playback driven by the audio clock. Verify: `web/e2e/specs/preview-rust-playback.spec.ts` (new) passes.
- [x] 5. Add feature detection, the fallback and the setting. Verify: the existing preview specs pass with either executor forced.
- [ ] 6. Build the parity harness over the golden projects. Verify: `web/e2e/specs/preview-export-parity.spec.ts` (new) passes for every golden project within the ADR-0029 tolerance.
- [ ] 7. Run the Rust gate. Verify: `cargo fmt --check && cargo clippy -- -D warnings && cargo test && cargo deny check` in `renderer/` exits 0.
- [ ] 8. Measure playback on the reference project with every agent stopped (ask the orchestrator for the idle window). Verify: the numbers for both executors are in the hand-off.

## Test strategy

- Unit (Vitest): protocol messages, the frame cache, seek logic.
- End to end (Playwright, Chromium): seek, playback, fallback, parity against the export.
- Rust: the existing gate and the renderer's own goldens.
- Performance: measured, not asserted in this epic; epic 42 holds the budgets.

## Acceptance criteria

1. With the new executor forced, the editor's existing preview specs pass.
2. For every golden project, preview frames match the export within the ADR-0029 tolerance (`preview-export-parity.spec.ts`).
3. On a browser without the required features the editor still previews, with the current executor, and says which feature is missing.
4. `bun run typecheck`, `bun run check:thin-client`, `task test:web` and the Rust gate pass.
5. The `.wasm` digest is pinned and `bash scripts/build-renderer.sh` reproduces it.
6. The playback measurements for both executors are recorded.

## Risks and mitigation

- **Webview differences between systems.** Mitigation: feature detection and the fallback; only Windows is asserted here.
- **Performance below the current executor.** Mitigation: measure both; if the new one is slower on the reference project, it ships behind the setting, off by default, and the hand-off says so.
- **STOP** before adding a crate or an npm package, and if the plan format must change in a way that affects the export.

## Size

L

## Hand-off for review

The diff of `epic/26-browser-preview-on-the-rust-renderer`; parity results per golden project; the measurements; which browsers were exercised.
