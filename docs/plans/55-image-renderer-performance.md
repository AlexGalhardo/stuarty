# 55: Image renderer performance (NFR-061, the renderer half)

## Goal

A creator editing a large image document (4,000 by 4,000 pixels, 30 layers) sees the preview update within 100 ms after an edit and exports it within 2 s, the target that epic 54 brought from half a minute to seconds but did not reach.

## Context and related decisions

- PRD-004 NFR-061: a 4,000 by 4,000 pixel image document with 30 layers is composed for the preview within 100 ms after an edit and exported within 2 s on the reference machine.
- Author decisions D-113 (the requirement is not relaxed) and D-118 (epic 54 is integrated with what it reached; the rest moves here).
- Where epic 54 left it (SDD-021 section 13.2, measured on a loaded machine): a cold export of the reference document about 11 s, an edit of the top layer about 0.5 s over the cached picture of the layers below. The looks are about 90% of the composition: `sample` and `mix_pixel` about 45%, the box blur passes about 25%, mask coverage about 4%. The top layer alone on an empty canvas costs 0.36 s in the module; the rest of an edit is the instance, the upload of a 64 MB picture and two copies of it.
- Tried by epic 54 and measured with no gain, so not repeated: a box blur with a reciprocal and a row-by-row column pass, `WithMemoryCapacityFromMax`, `+simd128` in the WebAssembly build.
- Left by the review of epic 54: a render abandoned by a cancelled frame (`renderDetached`) runs to the end and its picture is not cached, so fast edits of a cold document compete with the current frame (cache it, or bound the detached renders); a cache hit does not call `AssetPath` and `LUTPath`, so an asset the user lost access to stays in the preview until its picture leaves the cache (the export, with no cache, refuses it); the `engine/export` package takes about 12 minutes on a loaded machine, past the default `go test` timeout.
- ADR-0029 (one renderer, Rust compiled to WebAssembly, run through `wazero`), ADR-0038 (image documents on the same core and renderer), `.agents/rules/rust.md`, `.agents/skills/renderer-parity/`.

## Scope

- Modules: `renderer/` (the layer preparation and the composition), `engine/export/image*.go` and `engine/renderhost/` where the host side is the cost, `service/image_view.go` for the editor frame.
- Prepare a layer (blur, sharpen, glow, shadow) only for the region a draw touches, so that an export can be drawn in bands, one instance per band, in parallel.
- Keep the prepared picture of a layer, so that a change of its opacity or position does not prepare it again.
- Cut the host cost of an edit: no full-frame upload and copies when only the top layer changes.

## Out of scope

- Changing NFR-061, the reference document or the reference machine.
- A GPU path, new image features, a new crate or Go module (a STOP for the author).

## Dependencies

54 (merged). Not in progress together with an epic that changes `renderer/` or `engine/export/image*.go`.

## Technical design

- **Bands.** The plan already gives each layer its rectangle; the renderer receives a band rectangle and prepares each layer only on the band plus the reach of its looks (a blur radius widens the band). Bands are drawn on separate instances and joined; the joined picture equals the picture drawn whole.
- **Prepared layers.** The prepared picture of a layer is keyed by the content of the layer without its opacity and position, inside the bounded cache of SDD-021 section 6.1, under the same memory limit and the same scope per user.
- **Parity.** Every image golden file and renderer parity test stays unchanged; a speed-up that changes a pixel is a defect.

## Tasks

- [ ] 1. Specify bands and prepared layers in SDD-021 first. Verify: `task lint:md` exits 0.
- [ ] 2. Layer preparation limited to a band, and the export drawn in parallel bands. Verify: the Rust gate of `.agents/rules/rust.md` and `go test ./engine/export/... -run 'TestImage'` pass, golden files unchanged.
- [ ] 3. The prepared picture of a layer kept across an edit of its opacity or position. Verify: a test that such an edit prepares nothing again, and the cache tests pass.
- [ ] 4. The editor frame of an edit without a full-frame upload. Verify: `go test ./service/ -run Image` passes and `BenchmarkImageReference` reports `compose_ms` ≤ 100.
- [ ] 5. Measure on the idle machine (all agents stopped). Verify: `go run ./internal/tools/perfreport -write -absolute -gated -only NFR-061` exits 0.

## Test strategy

The golden files and renderer parity tests unchanged; a test that the picture drawn in bands equals the picture drawn whole, for every look and blend mode; cache tests for the prepared layers (hit, miss, limit, no sharing across users); the benchmark of epic 54.

## Acceptance criteria

1. `go run ./internal/tools/perfreport -write -absolute -gated -only NFR-061` on the idle reference machine exits 0: `compose_ms` ≤ 100 and `s_export` ≤ 2 (the orchestrator runs it with every agent stopped).
2. Every image golden file and renderer parity test passes unchanged; the Rust gate passes.
3. Exporting the reference document twice gives byte-identical files.
4. `task lint` and `task test` pass; no new dependency.

## Risks and mitigation

- Parallel bands need several renderer instances in memory: the number of bands is bounded by the memory limit of SDD-021 and by the CPU count.
- If 100 ms stays out of reach on the CPU, the senior stops with the profile and the question goes to the author.

## Size

L

## Hand-off for review

The diff against the release branch, the profile before and after, the commands of tasks 2 to 4 with their results, and the numbers of task 5 measured by the orchestrator on the idle machine.
