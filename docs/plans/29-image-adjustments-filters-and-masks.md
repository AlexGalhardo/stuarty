# 29: Image adjustments, filters, masks and blend modes

## Goal

A creator makes a photo pop behind a title: brighter, more contrast, a blurred background, a cut-out shape, a glow on the text. The part of an image editor most people use every day, and nothing more.

## Context and related decisions

- ADR-0038, ADR-0029, ADR-0003.
- Author decision D-062 (the most used fifth of an image editor, on the same core). PRD-004 RF-333, RF-334, RF-335, NFR-061, NFR-062.
- SDD-013 section 3 (the pixel contract: colour matrix, blur, blending), SDD-021. Read `.agents/rules/rust.md`, `.agents/skills/renderer-parity/` and `.agents/skills/deterministic-media/`.

## Scope

- Modules: `core/image/` (the commands and their validation), `renderer/vukem-renderer/` and `renderer/vukem-renderer-gpu/` (only operations the pixel contract lacks), `engine/renderhost/`, SDD-013 (new contract entries), SDD-021, and the inspector of epic 28 when it is merged (controls for the new commands).
- **Adjustments** (RF-333): brightness, contrast, saturation, temperature, tint, hue, exposure, levels (black point, white point, gamma), a tone curve of up to eight points.
- **Filters** (RF-334): gaussian blur, sharpen, vignette, grain, drop shadow, outer glow, stroke; the colour filters and LUT the video core already has.
- **Masks** (RF-335): shape mask (rectangle, rounded rectangle, ellipse) with feather and invert; a mask from another layer's alpha (clipping).
- **Blend modes**: normal, multiply, screen, overlay, soft light, hard light, darken, lighten, colour dodge, colour burn, difference.

## Out of scope

- Mask painting with a brush, selections by colour, content-aware anything.
- Liquify, perspective warp, lens corrections, HDR.
- Operations that need a model (epic 30).

## Dependencies

27.

## Technical design

- **Reuse first**: the video core already has brightness, contrast, saturation, temperature, hue, colour filters, LUT, blur, shadow, outline, vignette, grain, five blend modes and shape masks with feather. Each is exposed to image layers through the same types; only what is missing is new: exposure, levels, the tone curve, sharpen, outer glow, clipping masks, six blend modes.
- **Commands**: `layer.adjust`, `layer.filter_add`, `layer.filter_set`, `layer.filter_remove`, `layer.filter_move`, `layer.mask_set`, `layer.mask_clear`, `layer.blend`. Filters are an ordered list per layer; the order of operations is the one of SDD-013 section 3.3, extended in the specification before code.
- **Pixel contract**: every new operation is specified in integer or fixed-point arithmetic in SDD-013 before it is written (the tone curve as a 256-entry table built in Go and passed in the plan; levels as a table; sharpen as an unsharp mask on the existing blur; the new blend modes with their exact formulas and rounding). The CPU path is the truth; the GPU path matches within the ADR-0029 tolerance.
- **Available to video too**: because the renderer and the plan are shared, each new operation is also registered for video clips when it makes sense, with one golden case, so the two domains do not drift.
- **Determinism**: golden frames per operation on the CPU path; byte-identical exports.
- **Performance**: NFR-061 is checked on the reference document (4,000 by 4,000 pixels, 30 layers) and the number recorded; blur radius and the number of filters per layer have validated ceilings.

## Tasks

- [x] 1. Specify each new operation in SDD-013 (formula, rounding, order) and the commands in SDD-021. Verify: `task lint:md` exits 0.
- [x] 2. Expose the existing adjustments, filters, masks and blend modes to image layers with golden cases. Verify: `go test ./core/image/... -run "TestAdjust|TestFilter|TestMask|TestBlend"` passes.
- [x] 3. Implement exposure, levels and the tone curve (tables built in Go) on the CPU path. Verify: `cargo test` in `renderer/` and `go test ./engine/renderhost/...` pass with new goldens.
- [x] 4. Implement sharpen and outer glow on the CPU path. Verify: the same commands pass with new goldens.
- [x] 5. Implement the six blend modes and clipping masks on the CPU path. Verify: the same commands pass with new goldens.
- [x] 6. Implement the GPU path for the new operations. Verify: the parity tests of `renderer-parity` pass within tolerance.
- [ ] 7. Register the operations that apply to video clips, with one golden case each. Verify: `go test ./core/command/...` passes with the updated registry snapshot.
- [ ] 8. Expose on the three surfaces and regenerate documents. Verify: `go test ./web/contract/...` and `task generate:check` pass.
- [ ] 9. Add the inspector controls when epic 28 is merged; otherwise leave the note. Verify: `web/e2e/specs/image-adjustments.spec.ts` passes, or the note is in the hand-off.
- [ ] 10. Run the Rust gate and record the NFR-061 number. Verify: `cargo fmt --check && cargo clippy -- -D warnings && cargo test && cargo deny check` exits 0; the number is in the hand-off.

## Test strategy

- Golden cases per command; golden frames per operation on the CPU path.
- CPU and GPU parity within tolerance.
- Determinism: identical bytes on two runs.
- Property tests for tables (monotonic curve input, clamping).

## Acceptance criteria

1. Every adjustment, filter, mask kind and blend mode listed in Scope is a command reachable on HTTP, CLI and MCP, with a golden case.
2. For every new operation the CPU output equals its golden frame exactly, and the GPU output is within the ADR-0029 tolerance.
3. Exporting a document that uses every new operation twice gives identical files.
4. The reference document composes and exports with the numbers recorded against NFR-061.
5. The Rust gate, `go test ./core/... ./engine/renderhost/... ./web/contract/...` and `task generate:check` pass.

## Risks and mitigation

- **Float arithmetic creeping into the CPU path.** Mitigation: the specification first; `clippy.toml` rules; goldens on three systems when epic 43 runs.
- **Scope growth toward a full image editor.** Mitigation: the Scope list is closed; anything else is a new epic.
- **STOP** before adding a crate.

## Size

L

## Hand-off for review

The diff of `epic/29-image-adjustments-filters-and-masks`; the new SDD-013 entries; goldens; parity results; the NFR-061 number.
