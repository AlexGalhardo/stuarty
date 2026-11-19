# 27: Image documents: core, catalogue, templates and export

## Goal

A creator's agent makes a thumbnail the same way it edits a video: by sending commands to a document, seeing a description of the result, undoing a mistake, and exporting a PNG. This epic builds that document with no screen at all; the screen is epic 28.

## Context and related decisions

- ADR-0038 (an image is a second kind of document on the same command core and the same renderer), ADR-0002, ADR-0005, ADR-0029.
- Author decision D-062. PRD-004 RF-330, RF-331, RF-332, RF-336, RF-337, RF-338, RF-339, NFR-061, NFR-062.
- SDD-013 (an image is a plan of one frame), SDD-002 (the form of a command catalogue). Research note `web/01-engine-imagem`. Read `.agents/skills/deterministic-media/`.

## Scope

- Modules: `core/image/` (new: the document, its commands, its lint, its plan), `core/command/` and `core/schema/` (registration in the registry, domain `image`), `engine/export/` (still export through the renderer host), `service/` (image operations), `web/app/` (controllers, CLI, MCP), `web/database/migrations/`, `docs/sdds/021-image-documents.md` (new), SDD-000, SDD-005.
- Layers: image (an asset), text, shape (rectangle, rounded rectangle, ellipse), solid fill, group. Per layer: transform, opacity, visibility, lock, name, crop of an image layer.
- Templates with slots, export to PNG, JPEG and WebP, lint.

## Out of scope

- Adjustments, filters, masks and the wider set of blend modes (epic 29).
- Operations with a model (epic 30). The UI (epic 28).
- Brushes, painting, selections by pixel, vector pen paths, RAW, CMYK, PSD import.

## Dependencies

03.

## Technical design

- **Document** in `core/image`: canvas size in pixels, background, an ordered tree of layers with stable ids. Coordinates are integers or exact rationals as in the timeline; no float decides a pixel.
- **Commands** follow the batch model of the video core unchanged: atomic batches, base revision, idempotency key, dry run, operation log, undo and redo per actor, trash, conflict detection per object. The first catalogue: `image.create`, `canvas.resize`, `layer.add_image`, `layer.add_text`, `layer.add_shape`, `layer.add_fill`, `layer.group`, `layer.ungroup`, `layer.move` (order and parent), `layer.transform`, `layer.set` (opacity, visibility, lock, name), `layer.crop`, `layer.duplicate`, `layer.remove`, `text.set`, `shape.set`, `layer.align` (to canvas or to another layer), `layer.fit` (cover, contain). Each has a JSON Schema and golden cases like SDD-002. `text.set` and `shape.set` already exist for video: an image operation takes the name of its video counterpart only when it shares the schema and the code, and the registry resolves it by the kind of the target document; every other image operation gets a name that does not exist yet. Task 1 fixes the names in SDD-000 before any code.
- **Reuse, do not fork**: text styling, shapes, transforms and brand kit values use the same types and validation as the video core; a shared type moves to a common package rather than being copied.
- **Plan**: `core/image` produces a one-frame plan in the format of SDD-013 section 2; the renderer needs no new concept for this epic.
- **Export**: the Go host renders the plan with the CPU path (the bit-exact one) and encodes PNG, JPEG (quality) and WebP; if an encoder needs a new dependency, that format waits and the STOP applies. Presets for the common sizes (a video thumbnail, a square post, a vertical cover) are data.
- **Templates**: a document with named slots (text, image); `ImageFromTemplate(template, values)` validates the values and yields a document.
- **Lint** (RF-339): text outside the canvas, text below a minimum size for the preset, low contrast between text and what is under it, an image layer scaled far above its source, an empty slot.
- **Text view**: `ImageDescribe` returns the layer tree with positions and the lint findings within the token budget.
- **Storage**: an image document is a project item of kind `image` when epic 06 is merged; until then it is stored by project id with the same tables, and the item is added by whichever epic merges second.

## Tasks

- [x] 1. Write SDD-021 (document, catalogue with schemas, plan mapping, export, lint) and the SDD-000 names. Verify: `task lint:md` exits 0.
- [x] 2. Implement the document and the first half of the catalogue (create, canvas, add, remove, order, group) with golden cases. Verify: `go test ./core/image/...` passes.
- [ ] 3. Implement transform, set, crop, align, fit, text and shape commands with golden cases. Verify: `go test ./core/image/...` passes.
- [ ] 4. Wire the shared batch machinery: log, undo and redo, conflicts, dry run. Verify: `go test ./core/image/... -run "TestUndo|TestConflict|TestDryRun"` passes.
- [ ] 5. Produce the one-frame plan and render it through the Go host. Verify: `go test ./engine/export/... -run TestImageRender` passes against golden PNG files.
- [ ] 6. Implement export formats and presets, and the determinism test. Verify: `go test ./engine/export/... -run TestImageExportDeterministic` passes (two runs, identical bytes).
- [ ] 7. Implement templates and the lint. Verify: `go test ./core/image/... -run "TestTemplate|TestLint"` passes.
- [ ] 8. Add storage and the service operations with the text view. Verify: `go test ./service/... -run TestImage` passes.
- [ ] 9. Expose on HTTP, CLI and MCP in the `image` domain with parity. Verify: `go test ./web/contract/...` passes and `task generate:check` exits 0.
- [ ] 10. Write the reference scenario of an image (a thumbnail from a template, by CLI and by MCP). Verify: `bash scripts/smoke-e2e.sh` (extended) exits 0.

## Test strategy

- Golden cases per command, as in the video catalogue.
- Golden frames for rendering; byte-identical exports for determinism (NFR-062).
- Contract: parity; the frozen surface gains only additions.
- Performance (NFR-061) is measured by epic 42; this epic records a first number.

## Acceptance criteria

1. An agent can create an image, add an image layer, a text and a shape, align them, undo one step and export a PNG, through CLI and through MCP.
2. Exporting the same document twice produces identical files (NFR-062).
3. Every command of the catalogue has a schema, golden cases and is reachable on the three surfaces.
4. `ImageDescribe` returns the layers and the lint findings inside the token budget.
5. A template with a missing slot value is refused with a stable error; a filled one exports.
6. `go test ./core/image/... ./engine/export/... ./service/... ./web/contract/...` and `task generate:check` pass.

## Risks and mitigation

- **A second core that drifts from the first.** Mitigation: shared types and the shared batch machinery; a review rule in SDD-021.
- **An encoder dependency.** Mitigation: PNG and JPEG from the standard library; WebP only with an approved, permissively licensed encoder. **STOP** otherwise.
- **STOP** if the plan format of SDD-013 cannot express a layer without a change that touches the video path.

## Size

L

## Hand-off for review

The diff of `epic/27-image-document-core`; the catalogue table; golden outputs; the outputs of the acceptance commands; the first performance number.
