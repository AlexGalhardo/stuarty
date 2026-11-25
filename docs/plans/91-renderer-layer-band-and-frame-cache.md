# 91: Image renderer: prepare only the visible band, keep the prepared layer, cache the editor frame

## Goal

A creator exports a 4,000 by 4,000 pixel image document with 30 layers in at most 2 seconds and sees the editor picture of an edit within 100 ms on the reference machine, the budget of NFR-061 that epics 54 and 55 brought from 45 seconds to 2.5 seconds but did not reach.

## Context and related decisions

- PRD-004 NFR-061: a 4,000 by 4,000 pixel image document with 30 layers is composed for the preview within 100 ms after an edit and exported within 2 s on the reference machine. Author decisions D-113 (the requirement is not relaxed), D-118 (epic 54 hands its rest to epic 55) and D-135 (item 5): a second epic in `renderer/` that does not prepare layers outside the visible band, keeps the prepared layer, and adds a cache in `ImageFrame`.
- Where the code stands today (read before this epic was written; task 1 checks it again). Epic 55 tasks 1 to 4 are merged, task 5 measured on the idle reference machine (AMD Ryzen 7 5700X3D, 2026-11-21, `docs/perf/2026-11-21-windows.md`): `compose_ms` 51 ms (limit 100, passes) and `s_export` 2.50 s (limit 2.00, fails). SDD-021 section 6.2 records what epic 55 built, and that it **departed from the band-limited preparation its plan started from**:
  - A layer is prepared whole, once (`vr_prepare`, `engine/export/image_bands.go` `prepareOne`), on up to 8 workers, and only the draw is cut in bands (`drawBands`, `rowsOf`). A layer that reaches past the canvas, or that a band does not touch, is still prepared in full: its looks (blur, sharpen, glow, shadow) run on pixels no band draws.
  - The prepared picture is cached under the key of the layer without its opacity, position, blend mode and mask (`preparedKey`), so an edit of those prepares nothing again. A position that is not a multiple of half a pixel stays in the key.
  - `service.ImageFrame` (`service/image_view.go`) already passes one `ImageCache` per service (`s.imageFrames`, scope `Caller.OwnerUserID`) to `export.RenderImage`; the cache holds composed pictures of the layers below the top one and the prepared layers. What it does not cache is the answer: every `ImageFrame` call draws the canvas at its full size and `EncodeImage` resamples it to the requested width, so asking again for the same revision at the same width pays the render and the encode again.
  - Longest task of a cold export: one rotated 2,000 by 2,000 photo with tone, colour matrix and sharpen prepares in about 0.43 s in the module (0.17 s native), and the workers wait for it (SDD-021 section 13.3).
- Tried by epics 54 and 55 with no gain, so not repeated: a box blur with a reciprocal and a row-by-row column pass on the old path (the version that shipped uses the channels that can be non-zero), `WithMemoryCapacityFromMax`, `+simd128` in the WebAssembly build.
- ADR-0029 (one renderer, Rust compiled to WebAssembly, run through `wazero`), ADR-0038, `.agents/rules/rust.md` (the CPU path is the truth and bit-exact: integer or fixed-point arithmetic for pixels, no result that depends on the thread count), `.agents/skills/renderer-parity/`, SDD-013 section 4.2.2 (`vr_prepare`, `vr_put_prepared`), SDD-021 sections 6.1 and 6.2, SDD-008 (the NFR-061 row).
- Honest note for the author: the three levers of D-135 item 5 partly exist already (the cache in `ImageFrame`, the kept prepared layer, the bands of the draw). This epic is written around what is still missing in the code; task 1 measures which of the levers below still buys time before any is built, and a lever that does not is recorded as skipped, not forced.

## User stories

- As a creator, I want a large thumbnail or poster to export in a couple of seconds so that I can iterate on it.
- As a creator, I want reopening an image, or asking for the same picture again, to be instant.
- As the team, I want NFR-061 met as written on the idle reference machine, not relaxed.

## Scope

- Modules: `renderer/` (the preparation of a layer for a window of the canvas), `engine/renderhost/` (the host side of the new input of `vr_prepare`), `engine/export/image*.go` (the windowed preparation, the cache key, the encode), `service/image_view.go` (the frame cache), SDD-013 section 4.2.2, SDD-021 sections 6.2 and 13, SDD-008 (the row of NFR-061 if the method changes), `docs/perf/`.
- Must not touch: `core/image` (the plan and the pixels it describes do not change), the web UI, the Go video compositor, `capture/`.

## Out of scope

- Changing NFR-061, the reference document or the reference machine; a GPU path for the image export; new image features; a new crate or Go module (a STOP for the author); rendering the editor picture at a lower scale than the export (it would not be the same pixels as the export, and the preview must match it).
- Fast paths for the video renderer.

## Dependencies

54 (done), 55 (tasks 1 to 4 merged; its task 5, the idle measurement, is still owed and this epic's task 7 takes it over: the orchestrator marks epic 55 done when that measurement passes). Not in progress together with an epic that changes `renderer/` or `engine/export/image*.go`, nor with epic 93 once 93 re-pins the module (see 93).

## Surfaces

| Surface | Change |
| --- | --- |
| Web and desktop | None to the code; the editor frame is faster and repeated frames are instant |
| HTTP, CLI, MCP | None: `ImageFrame` and `ImageExport` keep their requests and their results byte for byte |

## Core commands and permissions

None. No command, no permission changes.

## Technical design

Three levers, each guarded by the measurement of task 1 (a lever whose share of the 2.5 s is under about 10% is recorded as skipped in SDD-021):

- **A. Prepare only the visible window.** `vr_prepare` takes an optional window (a rectangle of the canvas, in output pixels) and returns only the prepared pixels the window can show: the layer rectangle intersected with the window, widened by the reach of its looks (a blur radius widens it by three radii on each side, a shadow by its offset and blur, a glow by its radius, a sharpen by its radius) so that the pixels inside the window are exactly the ones a whole preparation gives. The block says which part of the layer it holds. A layer outside the window prepares nothing. The export asks for one window per band for a layer much larger than its band or partly outside the canvas, and for the whole layer otherwise; the editor frame keeps whole preparation (a drag changes which window shows, and a whole picture stays valid across it). Equality is the rule: the draw from a windowed picture is byte-identical to the draw from a whole one.
- **B. Keep the prepared layer across the edits an interactive session makes.** The key of `preparedKey` stays without opacity, position, blend mode and mask. The profile of task 1 lists which interactions still prepare a layer again (a resize, a rotation, a change of a look) and their cost on the reference document; where a change of a parameter only changes the draw (the rotation of an already prepared picture that the module could draw rotated, a mask), the preparation moves to the draw step, with the same pixels. A held window (lever A) is reused when it contains the window asked for.
- **C. Cache the answer of `ImageFrame`.** A bounded cache of encoded frames in the service, keyed by the owner (`Caller.OwnerUserID`, empty in local mode), the document digest of the revision (`doc_sha256` of the row, the canonical SHA-256 that already identifies the content) and the width. It is under its own memory limit (proposed 64 MiB, least recently used first), is never shared across users (T-89), and holds nothing on disk. A hit does not skip the access check: it still asks for the file of every image and lookup table of the visible layers, as SDD-021 section 6.2 requires of a cache hit, so an asset the caller lost access to is refused. A cache of encoded frames never answers for a different revision or a different asset content: the digest names the content and the asset hashes are in the document.
- **Parity.** Every image golden file and renderer parity test stays unchanged; a speed-up that changes one pixel is a defect. Determinism (NFR-062): exporting the reference document twice gives the same bytes, and the same bytes whatever the number of workers or bands.
- **Measurement.** Only the orchestrator measures against the budget, with every agent stopped and the machine idle (`go run ./internal/tools/perfreport -write -absolute -gated -only NFR-061`). The senior measures shares and direction on a loaded machine and never writes a budget number from one.

## Tasks

- [x] 1. Profile the cold export and the edit of the reference document again on the current tip, stage by stage and layer by layer (time spent preparing pixels outside the canvas or outside the band that draws them, time of the longest layer, the interactions that re-prepare a layer, the cost of an `ImageFrame` repeated at the same revision and width). Write the table in SDD-021 section 13.4 with the decision for each lever (build it, or skip it with its share). Verify: `go test -run '^$' -bench BenchmarkImageReference -benchtime 5x ./engine/export/` prints `compose_ms` and `s_export`, and `task lint:md` exits 0.
- [x] 2. Specify first: SDD-013 section 4.2.2 (the optional window of `vr_prepare`, the block that says what part it holds, the reach of each look), SDD-021 sections 6.2 and 6.1 (the window in the key, the frame cache and its access check and limit). Verify: `task lint:md` exits 0.
- [x] 3. **Skipped by the profile of task 1 (SDD-021 section 13.4: 0.2% of the prepared pixels lie outside the canvas, so a window has no work to cut); `vr_prepare` and the module are unchanged, and the Rust test that a prepared layer turned is the layer turned (`tests/prepared.rs`, which lever B stands on) was added.** Lever A in `renderer/`: the windowed preparation, for every look, with a test per look that the windowed picture equals the whole one inside the window (blur, sharpen, glow, shadow, outline, tone, colour matrix, lookup table, filters, rotated and unrotated, a layer past each side of the canvas). Verify: `cargo fmt --check && cargo clippy --locked --all-targets -- -D warnings && cargo test --locked && cargo deny check` in `renderer/` exit 0, and `bash scripts/build-renderer.sh embed` rebuilds the module reproducibly.
- [x] 4. **Skipped as written (no window to ask for, task 3); what the profile asked for instead was built: the bands are cut where the work is, not where the rows are equal (SDD-021 sections 6.2 and 13.4), and a test that the reference document is the same bytes for 1, 2 and 8 workers.** Lever A in the host and the export: ask for a window per band when the profile says it pays, join the bands, keep the key and the cache rules. Verify: `go test ./engine/renderhost/... ./engine/export/... -run 'TestImage|TestPrepared'` pass with every golden file unchanged, and `TestImageBandsEqualTheWholeRender` also covers the windowed path.
- [x] 5. **Built for the rotation, the one interaction that prepared a layer again for no pixel (SDD-021 section 13.4); the held window is moot with no window.** Lever B: move to the draw whatever the profile shows can move without a pixel changing, and extend the cache so that a held window is reused when it contains the one asked for. Verify: a test that an edit of each interaction in the profile prepares nothing, or only the layer it changes, and `go test ./engine/export/... -run 'TestImageCache'` passes.
- [x] 6. Lever C: the frame cache in `service.ImageFrame`, with its limit, its scope and its access check. Verify: `go test ./service/ -run 'ImageFrame'` passes, including a hit for the same revision and width, a miss after an edit and after an undo to another revision, no sharing across users in network mode, the refusal on a hit after the caller lost an asset, and the limit.
- [ ] 7. The idle measurement. Verify (run by the orchestrator, all agents stopped): `go run ./internal/tools/perfreport -write -absolute -gated -only NFR-061` exits 0 with `compose_ms` at most 100 and `s_export` at most 2; the table is committed under `docs/perf/`.
- [ ] 8. The documents: SDD-021 sections 6.2 and 13 (numbers and decisions), SDD-013, the roadmap note of epic 55. Verify: `task lint:md` exits 0 and `go test ./web/contract/...` passes.

## Test strategy

The golden files and the renderer parity tests unchanged; a Rust test per look that a windowed preparation equals the whole preparation inside its window; Go tests that bands built from windows equal the whole render for every look and blend mode and a `clip` source off the canvas; cache tests for the window containment, the scope and the limit; service tests for the frame cache; the benchmark for the shares; the orchestrator's idle run for the budget. No fixture is new: the reference document of `engine/export/image_reference_test.go` is the one of NFR-061.

## Acceptance criteria

1. `go run ./internal/tools/perfreport -write -absolute -gated -only NFR-061` on the idle reference machine exits 0: `compose_ms` at most 100 and `s_export` at most 2 (the orchestrator runs it with every agent stopped).
2. Every image golden file and renderer parity test passes unchanged; the Rust gate of `.agents/rules/rust.md` passes; `bash scripts/build-renderer.sh check` reports the embedded module is the one the source builds.
3. Exporting the reference document twice gives byte-identical files, and the same bytes for 1, 2 and 8 workers (a test).
4. `service.ImageFrame` answers a second request of the same revision and width without drawing (a counter in the test), never across users, never for an asset the caller lost, and within its memory limit.
5. SDD-021 section 13.4 holds the profile and says, for each lever, whether it was built or skipped and why.
6. `task lint` and `task test` pass; no new dependency.

## Risks and mitigation

- A window must include the reach of every look or the edge of a band shows a seam. Mitigation: the equality test per look, and a golden of a document whose blur reaches across a band edge.
- The 2 s budget may stay out of reach on the CPU through `wazero`. Mitigation: task 1 tells early where the time is; **STOP** if the three levers together cannot bring `s_export` under 2 s on the profile: the senior reports the profile and the question goes to the author (the options are a GPU export path, a different budget, or a native helper, all of them decisions for the author, D-113).
- A frame cache can show a stale picture or leak across users. Mitigation: the key is the content digest and the owner, and every hit repeats the access check.
- Any change under `renderer/` changes the module digest: this epic embeds the new module with `build-renderer.sh embed`, and the digest pin of epic 93 is repeated after it.
- **STOP** if a pixel of any golden file changes, if a new crate or Go module is needed, or if the measurement can only be taken on a busy machine.

## Size

L: a renderer feature with a proof of equality per look, its host, a service cache and an idle measurement.

## Priority

Should. NFR-061 is the one numeric requirement of the product that does not pass.

## Hand-off for review

The commits; the profile table and the decision per lever; the Rust gate and Go test results; the module digest after `embed`; the numbers of task 7 measured by the orchestrator on the idle machine; which levers were built and which were skipped.
