# 54: Image pipeline performance (NFR-061)

## Goal

A creator editing a large image document (a 4,000 by 4,000 pixel thumbnail or poster with 30 layers) sees the preview update right after each edit and exports it in a couple of seconds, instead of waiting half a minute for every change and three quarters of a minute for the file.

## Context and related decisions

- PRD-004 NFR-061: a 4,000 by 4,000 pixel image document with 30 layers is composed for the preview within 100 ms after an edit and exported within 2 s on the reference machine.
- Measured on the idle reference machine (AMD Ryzen 7 5700X3D, 2026-11-20, `docs/perf/2026-11-20-windows.md`): `BenchmarkImageReference` `compose_ms` 31,991 ms and `s_export` 44.83 s. Every other NFR passed.
- Author decision D-113: option (a), an epic that makes the image pipeline fast enough, measured against NFR-061 as written; the requirement is not relaxed.
- ADR-0038 (image documents on the same core and renderer), ADR-0029 (one renderer, Rust compiled to WebAssembly, run through `wazero` in the export), ADR-0003 (one plan, two executors), SDD-008 section 7 (the NFR-061 row) and section 13 (the idle measurement), SDD-021 (image documents).
- The code this epic starts from: `engine/export/image.go`, `image_encode.go`, `image_lut.go` (the export of an image document), `core/image/` (the document, its commands and its flattened view, `flat.go`), the renderer in `renderer/` and its host in `engine/`, and the benchmark `engine/export/bench_image_reference_test.go` with the reference document of `engine/export/image_reference_test.go`.

## Scope

- Modules: `engine/export/` (the image export path), the image preview path of the renderer host in `engine/` and of `web/resources/js/image-editor/canvas/`, `renderer/` only where the composition itself is the cost (rule `.agents/rules/rust.md`), `core/image/flat.go` only if the flattened view is the cost, SDD-021 and SDD-008 (the measured numbers and any change of method).
- Profile first, then remove the cost where it is: the likely candidates, to be confirmed by the profile, are a full recomposition of all 30 layers on every edit (compose only what changed: per-layer cache keyed by the layer's content revision, dirty rectangles), a CPU fallback where the renderer should be used, repeated decoding of the same assets, a PNG encode at the slowest compression level, and work done once per pixel in Go that the renderer does in bulk.
- The preview half in the browser (the Playwright spec of the image editor that SDD-008 names) is measured too and kept within the same 100 ms.

## Out of scope

- Changing NFR-061, the reference document or the reference machine.
- Removing the Go compositor for video (epic 47).
- New image features, new dependencies (a new crate or Go module is a STOP for the author).

## Dependencies

27, 29, 36, 49, 50 (the image editor, its looks and LUTs, its history): all merged. Not in progress together with an epic that changes `engine/export/image*.go` or the image canvas.

## Technical design

- **Profile.** `go test -run x -bench BenchmarkImageReference -cpuprofile` and `-memprofile` on the reference document, and the browser performance trace of the image editor spec. The profile, with the top costs and their share, goes into the hand-off and into SDD-021 as a short note.
- **Incremental composition.** The image plan already lists layers in order; each layer's rendered tile is cached by (document id, layer id, layer revision, canvas size, looks), and an edit recomposes only from the first changed layer up, over the cached result below it. The cache is bounded (memory limit written in SDD-021) and is never shared across users (network mode, T-89).
- **Export.** One composition through the renderer (no second pass), then one encode at a fast, specified compression level (the file stays lossless). The bytes of a document exported twice stay identical (determinism, SDD-004).
- **Parity.** The composed pixels stay identical to the current golden files (`engine/export/image_*_test.go` and the renderer parity tests); a speed-up that changes a pixel is a defect.

## Tasks

- [x] 1. Profile compose and export of the reference document; write the top costs in SDD-021. Verify: `task lint:md` exits 0 and the hand-off quotes the profile.
- [ ] 2. Incremental composition with the bounded per-layer cache, specified first in SDD-021. Verify: `go test ./engine/export/... -run 'TestImage'` passes and the golden files are unchanged.
- [x] 3. The export path: one composition and a fast lossless encode. Verify: `go test ./engine/export/... -run 'TestImage|TestSecurity_'` passes and an export twice gives the same bytes.
- [ ] 4. The browser preview of an edit within 100 ms on the reference document. Verify: the image editor performance spec passes in Chromium.
- [ ] 5. Measure on the idle machine (all agents stopped). Verify: `STUARTY_TEST_FFMPEG_DIR=... go run ./internal/tools/perfreport -write -absolute -gated -only NFR-061` exits 0 with `compose_ms` ≤ 100 and `s_export` ≤ 2.

## Test strategy

Unit tests of the cache (hit, miss, eviction under the memory limit, invalidation on an edit of a lower layer, no sharing across users), the existing image golden files and renderer parity tests unchanged, the export determinism test, the benchmark on the idle machine, and the browser spec.

## Acceptance criteria

1. `go run ./internal/tools/perfreport -write -absolute -gated -only NFR-061` on the idle reference machine exits 0: `compose_ms` ≤ 100 and `s_export` ≤ 2 (the orchestrator runs it with every agent stopped).
2. Every image golden file and renderer parity test passes unchanged: `go test ./engine/export/... ./core/image/...` and the renderer gate of `.agents/rules/rust.md` when `renderer/` changed.
3. Exporting the reference document twice gives byte-identical files (a test).
4. The per-layer cache has a memory limit written in SDD-021 and a test that it is not shared across users in network mode.
5. `task lint`, the type checks and `task test:web` pass; no new dependency.

## Risks and mitigation

- The cost is in the renderer itself: then the work moves to `renderer/`, under the Rust gate and the parity skill.
- 100 ms is out of reach for a full recomposition of 30 layers at 16 megapixels on the CPU: the incremental path makes the common edit (one layer) the measured case; if even that misses, the senior stops with the profile and the orchestrator asks the author.

## Size

L

## Hand-off for review

The diff against the release branch, the profile before and after, the commands of tasks 2 to 4 with their results, and the numbers of task 5 measured by the orchestrator on the idle machine.
