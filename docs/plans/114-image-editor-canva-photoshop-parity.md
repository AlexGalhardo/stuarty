# 114: Image editor parity with Canva and Photoshop: Magic Resize, gradients, curved text, smart guides, vector frames and AI retouching

## Goal

Creators using Stuarty's Image Editor get the everyday high-impact features they rely on most in **Canva** and **Adobe Photoshop** that Stuarty currently lacks—one-click **Magic Resize** across social aspect ratios, **linear and radial gradients**, **curved text and rich text backgrounds**, **smart alignment guides, magnetic snapping and equal distribution**, **vector shapes, Canva-style photo frames, flip and freehand brush masks**, and **AI Generative Fill, Magic Eraser, Magic Expand and Upscale**—all built cleanly on Stuarty's deterministic Go command core and single Rust WebAssembly renderer so preview, export, and AI agents stay in 100% parity.

## Context and related decisions

- Author decision D-160 (2026-11-27 product feedback on v0.60.0):
  - **Feedback #16 (Canva & Photoshop gap analysis)**: Researched the most heavily used everyday creator features in Canva and Adobe Photoshop (2025–2026) and audited them against `docs/sdds/021-image-documents.md` (Stuarty's 27 existing image operations):
    - **Already built in Stuarty**: layers/groups, basic crop, align, fit, brand kit, templates/slots, adjustments (brightness/contrast/saturation/temp/tint/hue/exposure/levels/curve/LUT), filters (blur/sharpen/vignette/grain/shadow/glow/stroke), rect/ellipse/layer masks, 11 blend modes, AI `remove_background`.
    - **Highest-demand features genuinely missing**:
      1. **Magic Resize / Multi-Aspect Reflow (Canva #1)**: today `canvas.resize` only keeps positions or naively stretches boxes without scaling text or reflowing layouts across 16:9 / 9:16 / 1:1 / 4:5 formats.
      2. **Linear & Radial Gradients (Canva & Photoshop)**: today `fill`, `shape`, and `text` only accept flat `#RRGGBB` solid colors.
      3. **Curved / Arched Text & Rich Text Effects (Canva)**: curve arc slider, text background pill/badge with padding & radius, letter spacing, line height, and gradient text fills.
      4. **Smart Guides, Magnetic Snapping & Equal Distribution (Canva & Photoshop)**: live center/edge/spacing snap lines while dragging on canvas + `layer.distribute` (horizontal/vertical equal spacing).
      5. **Vector Shapes, Photo Frames, Flip & Freehand Brush Masks (Canva & Photoshop)**: polygon, star, arrow, line shapes; Canva-style media clipping frames (circle, arch, badge, device mockup); horizontal/vertical flip; freehand brush/eraser mask paths for fine photo cutouts.
      6. **AI Generative Fill, Magic Eraser, Magic Expand (Outpaint) & Upscale (Photoshop Firefly & Canva Magic Studio)**: extending `providers.ImageEditor` (`SDD-016 §2.4`) beyond `remove_background` to cover the rest of creators' daily AI retouching workflow, plus a non-AI local clone/spot-heal stamp tool for quick offline blemish removal.
- ADR-0002 (all editing rules live in `core/image/`), ADR-0007 (thin UI client: smart guide snap targets may assist pointer dragging, while final coordinates are committed via core commands), ADR-0029 & ADR-0038 (one Rust renderer compiled to WASM for both browser preview and Go `wazero` export—follow `.agents/rules/rust.md` and `.agents/skills/renderer-parity/`!), ADR-0032 (AI retouching calls go through the approval queue & cost ledger), SDD-000, SDD-013, SDD-016, SDD-021.

## Scope

- Modules owned:
  - `docs/sdds/000-shared-definitions.md`, `docs/sdds/013-renderer.md`, `docs/sdds/016-ai-providers.md` §2.4, `docs/sdds/021-image-documents.md`.
  - `core/image/` (new deterministic operations & golden cases: `canvas.smart_resize`, `layer.distribute`, `layer.set_flip`, `layer.set_gradient` / gradient support in `add_fill`/`set_shape`/`set_text`, curved text & text-box pill styling in `layer.set_text`, expanded vector shapes [`polygon`, `star`, `arrow`, `line`, `frame_*`] in `add_shape`/`set_shape`/`mask_set`, freehand stroke mask paths in `layer.mask_set`, local spot-heal stamp command).
  - `renderer/` (Rust WASM renderer support for linear/radial multi-stop gradients, curved/arched text layout, polygon/star/arrow/line/frame geometry, horizontal/vertical flip, stroke-path raster masks—verified with deterministic golden frames and browser/export parity!).
  - `service/image*.go` and `service/providers/` (extend `ImageEditor` capability with `inpaint_fill`, `magic_eraser`, `outpaint_expand`, `upscale` using existing connected providers like OpenAI/fal/BFL/Recraft behind the approval queue and cost ceilings).
  - `web/app/` (HTTP, CLI, MCP parity for new image commands within NFR-020 budget).
  - `web/resources/js/image-editor/` (Magic Resize multi-format modal, Gradient picker, Curved Text & Text Pill controls, live Smart Guides & magnetic snapping overlay on canvas, Distribute toolbar actions, Shape/Frame picker, Flip buttons, Brush Mask & Spot Heal canvas tools, AI Retouching menu).
  - `web/resources/lang/en.json`, `web/resources/lang/pt-BR.json`.
- Must not touch: `core/timeline/`, `engine/export/` video encoder pipeline, `capture/`, `desktop/`.

## Out of scope

- Liquify freeform mesh warping and 3D modeling (can be evaluated later if creators ask).
- Local offline AI background removal model downloads (already scoped in epic 72).
- Video editor features (owned by epic 115).

## Dependencies

- Epics 27, 28, 29, 30, 86, 91, and 116 (image document core, UI, filters/masks, model operations, renderer frame cache, and versioned FigCraft design in `docs/design/final/stuarty-redesign-final-trilha.figcraft` — design new Magic Resize, Gradient, Curved Text, Frame, and Retouch controls in FigCraft). Coordinate `renderer/` changes sequentially with epic 115 (never two `renderer/` epics in progress together!).

## Technical design

1. **Magic Resize (`canvas.smart_resize`)**:
   - Pure deterministic algorithm in `core/image/`: computes proportional scale, safe-margin clamping, font-size scaling within readable min/max bounds, background `cover` fill expansion, and anchor-preserving reflow (top/bottom/center zones) when changing aspect ratio (e.g., `1280x720` -> `1080x1920` or `1080x1080`).
   - Optional batch variant helper creates linked resized copies for multiple social presets in one click (YouTube Thumbnail, Instagram Post, Instagram Story/Reels, TikTok Cover, LinkedIn Banner).

2. **Gradients, Curved Text, Vector Shapes & Brush Masks (`core/image/` + `renderer/`)**:
   - **Gradients**: `Gradient{Kind: "linear"|"radial", AngleDeg: 0..359, Stops: []{Offset: 0..1000, Color: "#RRGGBBAA"}}` (2 to 8 stops, integer thousandths for cross-machine determinism per ADR-0004/SDD-021!).
   - **Curved Text**: `curve_arc` (-360 to 360 integer degrees) + `bg_color`, `bg_padding`, `bg_radius`, `letter_spacing`, `line_height` on text style; glyph placement along circular arc computed deterministically in `renderer/stuarty-renderer-text/` so browser preview and `wazero` export are byte-identical!
   - **Vector Shapes & Frames**: extend shapes with `polygon` (`sides: 3..12`), `star` (`points: 3..12`, `inner_ratio`), `arrow`, `line`, and clipping frame masks (`arch`, `circle`, `star`, `badge`, `rounded_device`).
   - **Flip & Brush Mask**: `flip_x`, `flip_y` booleans on layers; `mask.kind = "strokes"` storing integer polyline brush/eraser strokes (`[{points, radius, erase, feather}]`) rasterized deterministically in Rust.

3. **Smart Guides & Distribution**:
   - `layer.distribute` (`layer_ids` >= 3, `axis`: `"horizontal"|"vertical"`, `mode`: `"edges"|"centers"`) lives purely in `core/image/`.
   - UI canvas renders live alignment/spacing guide lines (snapping within 6px to canvas center/edges and sibling layer edges/centers, bypassable by holding `Alt`) and commits exact integer coordinates via `layer.transform`.

4. **AI Retouching (`ImageEditor` extensions)**:
   - Extend `provider.EditRequest` operations (`remove_background`, `inpaint_fill`, `magic_eraser`, `outpaint_expand`, `upscale`), passing an optional mask PNG rendered from the user's brush mask selection, routed through `image.edit` approval-gated jobs and applied via `layer.replace_source`.

## Tasks

- [ ] 1. Specify all new image operations, gradient/curved-text/mask schemas, `ImageEditor` extensions, and renderer pixel contracts in SDD-000, SDD-013, SDD-016 §2.4, and SDD-021. Verify: `task lint:md` exits 0.
- [ ] 2. Implement core operations (`canvas.smart_resize`, `layer.distribute`, `layer.set_flip`, gradients, curved text & text pills, expanded vector shapes/frames, brush stroke masks) in `core/image/` with full golden cases and `task codegen`. Verify: `go test ./core/image/... ./core/schema/...` passes.
- [ ] 3. Implement gradients, curved text, new vector shapes/frames, flip, and brush masks in `renderer/`, rebuild WASM reproducibly (`bash scripts/build-renderer.sh`), and verify renderer parity + golden frames (`.agents/skills/renderer-parity/`). Verify: `cd renderer && cargo fmt --check && cargo clippy -- -D warnings && cargo test && cargo deny check` and `go test ./engine/export/... -run "Image"` pass.
- [ ] 4. Extend `service/` and `providers.ImageEditor` with `inpaint_fill`, `magic_eraser`, `outpaint_expand`, and `upscale` (behind approval queue & cost ceilings) with contract tests and HTTP/CLI/MCP parity. Verify: `go test ./service/... ./web/app/... ./web/contract/...` and `go run ./internal/tools/surfacebudget` pass.
- [ ] 5. Build the Image Editor UI controls (Magic Resize dialog, Gradient picker, Curved Text & Pill controls, Smart Guides & Snapping + Distribute, Shape/Frame picker, Flip, Brush Mask canvas tool, AI Retouching menu) and add `en.json` + `pt-BR.json` strings. Verify: `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n && bunx vitest run resources/js/image-editor/`.
- [ ] 6. Run Playwright E2E specs and renderer preview-vs-export parity verification for gradients, curved text, Magic Resize, smart guides, and brush masks. Verify: `task lint`, `task test`, `task test:web`, and the image editor Playwright specs pass.

## Test strategy

- Core golden tests: every new `core/image` command has success + error golden cases and undo/redo round-trip proof.
- Renderer parity tests: browser WASM frame vs Go `wazero` export frame within SDD-013 pixel tolerance for linear/radial gradients, curved text, star/arrow/frame masks, flip, and brush stroke masks; cross-machine determinism check.
- Service contract & security tests: AI retouching respects approval queue, cost ceilings, `accepts: ["image"]`, and `STUARTY_MAX_MEDIA_BYTES`.
- UI & E2E: Vitest tests for smart guide snapping geometry and controls; Playwright E2E resizing a 16:9 thumbnail to 9:16 via Magic Resize, applying a gradient + curved text, and exporting PNG.

## Acceptance criteria

1. A creator can run **Magic Resize** on a 16:9 thumbnail to produce a reflowed 9:16 story and 1:1 post with scaled typography and intact margins, and export byte-identical results matching preview.
2. Linear/radial multi-stop gradients, curved text with background pills, polygon/star/arrow/line shapes, Canva-style photo frames, flip H/V, and freehand brush masks all render deterministically with proven preview/export parity (`renderer-parity` skill passes).
3. Dragging layers on the Image Editor canvas shows live Smart Guides with magnetic snapping, and selecting 3+ layers enables one-click horizontal/vertical equal distribution (`layer.distribute`).
4. AI Retouching (`inpaint_fill`, `magic_eraser`, `outpaint_expand`, `upscale`) works through connected image-edit providers under the approval queue and cost ledger.
5. Rust gate (`cargo fmt --check && cargo clippy -- -D warnings && cargo test && cargo deny check`), `task lint`, `task test`, `task test:web`, `go run ./internal/tools/surfacebudget`, `cd web && bun run typecheck && bun run check:thin-client && bun run check:i18n`, and Playwright image-editor specs pass.

## Risks and mitigation

- **Rust renderer crate additions.** Mitigation: gradients, circular-arc glyph transforms, polygon/star rasterization, and stroke masks can be implemented directly in pure Rust using existing approved crates in `.agents/rules/rust.md`; **STOP** if any new crate is needed.
- **NFR-061 (100 ms preview / 2 s export budget).** Mitigation: keep brush masks and gradients inside the existing prepared-layer band cache (epic 91) and check performance regression before hand-off.
- **MCP budget (NFR-020).** Mitigation: new operations join the existing `image` command catalogue (`image_edit` batch tool), adding zero bytes to `tools/list`; verify with `surfacebudget`.

## Size

L

## Hand-off for review

The commits of `epic/114-image-editor-canva-photoshop-parity`, rebuilt deterministic WASM artifact + digest, Rust gate output, parity test results, `surfacebudget` output, Vitest/Playwright outputs, and before/after screenshots.
