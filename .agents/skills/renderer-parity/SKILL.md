---
name: renderer-parity
description: Prove that a renderer change keeps the preview and the export drawing the same frame. Use for any change to renderer/, engine/compositor, engine/raster, the preview executor, WebCodecs decoding or the golden frames (ADR-0029, Phase 5).
---

# Renderer parity

Read `rules/rust.md`, ADR-0029 and SDD-013 first. There are three gates, from strictest to loosest. A stage of Phase 5 becomes the default only when its gate passes on Linux, macOS and Windows, and in chromium, firefox and webkit where a browser is involved.

## 1. CPU path against the Go compositor: byte-equal

The Rust CPU path replaces `engine/compositor` and `engine/raster`, so for every golden case of `engine/export` it must produce the same bytes.

- Run the parity harness of P5-RND-003 natively and in WebAssembly (through `wazero`), on all three systems (CI matrix; locally only your own system counts).
- A single differing byte is a failure. Find the formula that differs (rounding, sampling, premultiplication, blend order); fix the port or, when the Go behaviour is the bug, fix both and regenerate the goldens in one reviewed commit that says why.
- Never regenerate goldens to make a parity test pass. `UPDATE_GOLDEN=1` is for an intended, reviewed change of output.

## 2. GPU path against the CPU path: SSIM 0.99, 0.5 pixel

- For every golden case, render with the GPU path and compare with the CPU path: SSIM on luma of at least 0.99 and at most 0.5 pixel of displacement.
- Record the adapter (vendor, backend: Vulkan, Metal, D3D12, WebGPU in which browser, or the software adapter) next to each result. A case that passes on one adapter and fails on another is a failure.

## 3. Preview against export in the browser

- `web/e2e/specs/preview-vs-export.spec.ts` in chromium, firefox and webkit, with `KNOWN_DIVERGENCES` empty. Until stage D the tolerance is ADR-0003 item 6 (SSIM 0.97, 1 pixel); from stage D it is the GPU tolerance above.
- Build the e2e bundle fresh (`bun run build:e2e`) before trusting a result: a stale bundle once gave SSIM 0.0003 on every case.
- Run each renderer and decoder variant the stage adds (WebGPU and WebGL; WebCodecs and `<video>`), forced by their test settings.
- For flakiness, repeat (`--repeat-each 500` on the golden cases for the black-texture check of P2-QA-003). An intermittent failure is a defect, never a retry.

## Performance

Budgets (NFR-001, NFR-003, NFR-004, NFR-007, NFR-009, export speed within 20% of the Go compositor) are measured only with every agent stopped and the machine idle, and recorded with the machine and the command. A number measured under load is not evidence either way.

## Report

For each gate: the command, the systems and browsers, the number of cases, the worst SSIM and displacement, and any case that failed with its cause.
