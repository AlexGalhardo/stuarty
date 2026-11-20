# 52: Live composition groups in the browser preview

## Goal

A creator who sets an opacity, a blend mode or an effect on a nested composition sees it while the timeline plays, drawn the way the export draws it, and not only when the playhead is paused.

## Context and related decisions

- ADR-0029 (one Rust renderer for the preview and the export), ADR-0013 (the browser preview stack), ADR-0003, ADR-0042.
- Author decision D-094. PRD-001 RF-083 (nested compositions).
- Epic 32 (the composition and its group, which the plan carries). Its task 1 writes the pixel rules of the group into SDD-013. **At the time of writing those rules are not in SDD-013 on `release/0.30.0`**: section 3 of SDD-013 ends at 3.11, so the section number is not known yet. This epic cites the section that epic 32 adds, and the number is confirmed when epic 32 is merged.
- Epic 26 (the browser worker and the Rust module in the preview, done).
- SDD-013 section 2.2 (the layer kinds of the plan: `media` and `bitmap`; any other kind is `unsupported_layer`), section 3.3 (the order of one frame), section 4.3 (one render, in order), section 4.7 and section 4.7.1 (the browser worker as implemented: the CPU path only; the GPU path is not implemented), section 4.9.
- SDD-004 section 10 (the preview executor), SDD-012 (the preview), SDD-000 section 12 (the renderer names).
- The code this epic builds on: `web/resources/js/editor/preview/RustExecutor.ts`, `web/resources/js/editor/preview/PixiExecutor.ts`, `web/resources/js/editor/preview/pausedFrame.ts` (the paused frame asks the server for `ViewFrame`, `service/view.go`) and `web/resources/js/editor/preview/render/`.

## Scope

- Modules: `web/resources/js/editor/preview/` (`RustExecutor.ts`, `render/` in the same folder, and `PixiExecutor.ts` for the fallback), `renderer/stuarty-renderer/` and `renderer/stuarty-renderer-gpu/` only if the group rule needs a module export that does not exist (task 1 decides), `scripts/build-renderer.sh` (only if the module changes), `web/e2e/specs/preview-*.spec.ts`, SDD-013 (section 4.7.1), SDD-012 (the preview section).
- The browser draws a group that epic 32 puts in the plan, live during playback, with its own opacity, blend mode, effect or mask, through the intermediate surface that the group rule of SDD-013 specifies.

## Out of scope

- The GPU path of the browser (SDD-013 section 4.7.1: not implemented; `prefer: "gpu"` is answered with the CPU path). It stays as it is.
- Time remapping of a group (epic 53) and groups deeper than the limit of epic 32.
- Removing the PixiJS executor and the Go compositor (epic 47).
- Any change to the export or to the format of the plan: both are epic 32's.

## Dependencies

32 (the group in the plan and its pixel rules; in progress; this epic cannot start before epic 32 is merged) and 26 (the browser worker, done).

Not in progress together with 51 (both change `web/resources/js/editor/preview/` and SDD-012).

## Technical design

- The plan of a window already carries the group (epic 32). The browser expands it in `web/resources/js/editor/preview/render/` into the render input of the module: the child layers are delivered as the layers of SDD-013 section 4.7.1 (a `texture` each), drawn to an intermediate surface the size of the group's box, and then placed as one layer with the group's opacity, blend mode, effect or mask, as the group section of SDD-013 says.
- **Intermediate surface.** A surface the size of the box, pooled in the worker. A surface of 4,096 by 4,096 pixels is 64 MiB (4096 x 4096 x 4 bytes). The number of surfaces alive in one frame is bounded by the depth of groups that epic 32 allows (four).
- **Parity.** The same frame of the same plan gives the same bytes as the export: SDD-013 section 4.7.1 holds the CPU path bit for bit. `web/e2e/specs/preview-export-parity.spec.ts` is extended with a nested project of two levels.
- **The PixiJS fallback.** When the Rust executor cannot run, the PixiJS executor draws the group with its own containers (alpha, blend mode, filters). That is not bit-exact. The diagnostic line says so, and parity is held only by the Rust executor (ADR-0029).
- **Playback.** The group is drawn live: no `ViewFrame` request while the timeline plays. `pausedFrame.ts` still asks the server when the playhead is paused, as it does today.

## Tasks

- [x] 1. Read the group rule that epic 32 wrote into SDD-013, and list in SDD-013 section 4.7.1 what the browser now does for a group and what it still does not. Verify: `task lint:md` exits 0. If the rule is not in SDD-013, **STOP**.
- [x] 2. Expand a group of the plan into the render input (in `web/resources/js/editor/preview/render/`), with Vitest cases for the names and shapes the module accepts. Verify: `task test:web` passes.
- [x] 3. The worker draws the group on the CPU path with the intermediate surface, and reproduces the golden frames of epic 32. Verify: `cargo test` in `renderer/` passes; `bash scripts/build-renderer.sh` reproduces the pinned digest, or updates it with the reason in the hand-off.
- [x] 4. Playback draws the group with no `ViewFrame` request. Verify: `web/e2e/specs/preview-nested-live.spec.ts` (new, `bun run e2e`) passes, including the assertion that no `/frame` request is made during playback.
- [x] 5. Parity of a two-level nested project with the export. Verify: `web/e2e/specs/preview-export-parity.spec.ts` passes with the nested project.
- [x] 6. The PixiJS fallback draws the group with the diagnostic line, or the preview says it cannot. Verify: `web/e2e/specs/preview-rust-playback.spec.ts` passes with the PixiJS executor forced.
- [x] 7. Typecheck and thin client. Verify: `bun run typecheck` and `bun run check:thin-client` pass.

## Test strategy

- Unit (Vitest): the expansion of a group; the pool of surfaces; the order of the layers inside a group.
- Golden: the golden frames of epic 32 for a group, reproduced by the worker.
- End to end (Playwright, Chromium): the playback of a group with an opacity and a blend mode; no server frame request during playback; parity with the export; the PixiJS fallback.
- Rust: the module's own tests and goldens (`cargo test`).
- Performance: the frame time with a group is recorded on the reference project. It is not asserted here; epic 42 holds the budgets.

## Acceptance criteria

1. During playback a composition with an opacity below 100 % and a blend mode is drawn as one unit: `web/e2e/specs/preview-nested-live.spec.ts` passes.
2. No `ViewFrame` request is made while the timeline plays: the same spec passes.
3. The preview of a two-level nested project matches the export within the tolerance of ADR-0029: `web/e2e/specs/preview-export-parity.spec.ts` passes.
4. Without the Rust executor the editor still previews, and it says that the group is drawn approximately: `web/e2e/specs/preview-rust-playback.spec.ts` passes with the PixiJS executor forced.
5. `bun run typecheck`, `bun run check:thin-client`, `task test:web` and `task lint:md` pass. The Rust gate, `cargo fmt --check && cargo clippy -- -D warnings && cargo test && cargo deny check` in `renderer/`, exits 0, and the pinned digest of `bash scripts/build-renderer.sh` is current.

## Risks and mitigation

- **The group rule is not written yet.** Epic 32, task 1, writes it. **STOP** if task 1 finds no group rule in SDD-013. This epic does not invent pixel rules in the browser.
- **A new module export or a new crate.** The group may need an export that the module does not have. **STOP** before a new crate (the approved list of `.agents/rules/rust.md`) or before a change of the module's contract (SDD-013 section 4.2).
- **Memory.** The intermediate surfaces are the canvas size. The pool is bounded, and tested at 4,096 pixels per side.
- **Exactness of the fallback.** The PixiJS path is approximate. If the author requires exact parity there, **STOP**: that question belongs to epic 47.

## Size

M

## Hand-off for review

The diff of `epic/52-live-composition-groups-in-the-browser`; the parity results for each golden project; the frame time with and without the group, recorded and not asserted; the decision on the module digest; which browsers were exercised.
