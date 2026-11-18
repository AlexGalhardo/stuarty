# 47: One renderer everywhere: remove the Go compositor and PixiJS

## Goal

Every pixel Stuarty shows or exports comes from one renderer, so a feature is written once and a difference between preview and export cannot exist by construction. The two older drawing paths are deleted.

## Context and related decisions

- ADR-0029 (one Rust renderer; the duplicates are removed after the proof on three systems), ADR-0038, ADR-0003.
- Author decision D-073. PRD-001 NFR-011; PRD-003 NFR-045.
- SDD-013, SDD-004, SDD-012. Read `.agents/rules/rust.md` and `.agents/skills/renderer-parity/`.

## Scope

- Modules: `engine/compositor/` (removed), `engine/export/` (the renderer host as the only compositor), `engine/rasterize/` (what the renderer's text crate replaces), `web/resources/js/editor/` (the PixiJS executor and its dependency removed), `web/package.json`, SDD-004, SDD-012, SDD-013, the notices.

## Out of scope

- New visual features.
- The GPU path in the export host beyond what SDD-013 already specifies.

## Dependencies

**Blocked until the three-system proof exists**: epic 26 merged, and epic 43 showing that the renderer's goldens pass and the preview runs in the system webview of Windows, macOS and Linux (or, where a webview lacks a feature, that the CPU path in the worker is fast enough to be the fallback).

## Technical design

- **The proof first**: a table per system (export goldens exact on the CPU path; preview parity within tolerance; playback performance of the reference project against the budget) taken from epics 26, 42 and 43. If any cell fails, this epic stops at task 1.
- **Export**: the Go compositor is removed only after every golden project exports identically through the renderer host, and the export performance budget holds. The switch is one commit that can be reverted; the deletion is a later commit.
- **Preview fallback**: with PixiJS gone, a browser without WebGPU or WebGL2 uses the renderer's CPU path in the worker. The measured frame rate of that path decides whether it is acceptable; if it is not, the fallback is a reduced-resolution CPU preview with a notice, never a second renderer.
- **Text**: if the renderer's text crate draws every text case the Go rasteriser draws (goldens), the Go rasteriser is removed too; otherwise it stays and the hand-off says what is missing.
- **Clean up**: dependencies that only the removed paths used leave `go.mod` and `web/package.json`; the notices are regenerated; the settings that chose an executor are removed with a migration that ignores the old value.

## Tasks

- [ ] 1. Build the proof table from the recorded results; STOP if a cell fails. Verify: the table is in SDD-013 and in the hand-off.
- [ ] 2. Make the renderer host the default compositor for export; keep the old one behind a build tag. Verify: `go test ./engine/export/...` passes with every golden identical.
- [ ] 3. Measure export performance against the budget on the idle machine (orchestrator). Verify: the number is in the hand-off and within the budget.
- [ ] 4. Delete `engine/compositor/` and what only it used. Verify: `go build ./... && task test` pass; `go mod tidy` leaves no stray requirement (`task tidy:check`).
- [ ] 5. Make the renderer worker the only preview executor, with the CPU fallback. Verify: the preview specs pass in Chromium, Firefox and WebKit.
- [ ] 6. Remove PixiJS and its code. Verify: `bun run typecheck`, `task test:web` and `task lint:web` pass; the package is gone from `web/package.json`.
- [ ] 7. Decide and apply the text rasteriser question. Verify: the text goldens pass on whichever path remains.
- [ ] 8. Regenerate notices and update the three specifications. Verify: `task notices:check` and `task lint:md` exit 0.

## Test strategy

- Every golden project, export and preview, before and after each removal.
- Three browser engines for the preview.
- The Rust gate and the full local gate at the end.

## Acceptance criteria

1. `engine/compositor/` and the PixiJS executor no longer exist, and nothing references them.
2. Every golden project exports identically to before the removal.
3. The preview works in the three browser engines, on the GPU path or the CPU fallback.
4. Export performance is within its budget.
5. `bash .agents/skills/verify/scripts/verify.sh --full` and the Rust gate pass.

## Risks and mitigation

- **Removing the fallback too early.** Mitigation: the proof table is task 1 and a STOP; the switch and the deletion are separate commits.
- **A webview with neither GPU path.** Mitigation: the CPU worker path, measured.

## Size

M

## Hand-off for review

The diff of `epic/47-one-renderer-everywhere`; the proof table; golden comparisons; the performance number; the text rasteriser decision.
