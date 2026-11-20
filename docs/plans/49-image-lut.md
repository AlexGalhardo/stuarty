# 49: Image lookup tables (.cube)

## Goal

A creator grades a photo with the same colour table (`.cube`) they use on a video, and gets the same colours in the editor's picture and in the exported PNG.

## Context and related decisions

- ADR-0038 (image documents on the same core and renderer), ADR-0029 (one renderer), ADR-0003 (the Go host and the bit-exact CPU path), ADR-0011 (only LGPL FFmpeg; the table is read by our own parser, `engine/lut`).
- Author decision D-093. PRD-004 RF-333 and RF-334. Epic 29 names the LUT among the colour filters of the video core that image layers should reuse.
- SDD-021 section 2 (`assets`), section 3.1 and section 14 decision 9 (the video lookup table is not exposed to an image: it "needs a `.cube` asset resolved by the service for a document"), section 5 (the plan of a look), section 12 (the limits).
- SDD-004 section 3.6 (the table: its arithmetic, its limits and the form the preview receives), SDD-013 section 2.2 (the `lut` field of a plan layer), section 3.3 step 5 (the order of a frame), section 3.4 (the LUT paragraph), section 4.2 (`vr_put_lut`), SDD-011 section 7.1 (a `lut` asset has no FFmpeg step), SDD-005 section 3.4 and section 5.5 (the normalized `.cube` artifact), SDD-007 section 12 (`FuzzCubeParse`).
- The code this epic builds on: `engine/lut/lut.go`, `engine/export/lut.go`, `renderer/stuarty-renderer/src/lut.rs`, `core/image/look.go`, `core/image/plan_look.go`, `core/image/cmd_look.go`.

## Scope

- Modules: `core/image/` (the `lut` key of `layer.adjust`, its validation and the plan field), `core/schema/image/` (regenerated), `service/image*.go` (resolving a `lut` asset of the store for a document), `engine/export/image*.go` (uploading the table before a layer is drawn), `web/resources/js/image-editor/inspector/` (the LUT control), `web/resources/lang/en.json` and `pt-BR.json`, SDD-021 (sections 2, 3.1, 5, 12 and 14), SDD-013 (sections 2.2 and 3.11), SDD-004 (section 3.6, one sentence).
- A LUT is a key of the look of a layer (`layer.adjust`), as a video clip has its `lut` (SDD-001 section 3.5). It is not a new kind of layer.

## Out of scope

- 1D tables, a LUT generator, colour management (ICC), a LUT on a group (a group has no look, SDD-021 section 2).
- A LUT in a browser preview of the image editor: the picture is the server's render (SDD-012 section 24.13, decision 1), so the screen follows the export.
- A GPU path for images: the image export is on the CPU path only (SDD-021 section 6).
- Any change to the `.cube` parser or to its limits.

## Dependencies

29 (the look of an image layer, done).

Soft dependency on 48 for the import route of a `.cube` file. Without 48 the file reaches the store as a video's LUT does (`MediaImport` in a project, SDD-011 section 7.1), and the image refers to it by its hash.

Not in progress together with 48 and 50: all three change `core/image` and SDD-021.

## Technical design

- **The key.** `layer.adjust` takes `lut` as `{asset, intensity}`. `asset` is the content hash (`sha256:...`) of a `lut` asset, which must be listed in the document's `assets` with `kind` `lut`. `intensity` is a whole percent from 1 to 100, default 100. `intensity` 0 or `reset` removes the key, as neutral values do (SDD-021 section 3.1). The entries of `assets` gain `kind`; an entry without it is an image, as today, so an existing document keeps its meaning.
- **Refusals**, as `clip.set_color` gives them for `lut_asset` (SDD-002 section B.5 and its errors): an asset that is not of kind `lut` is `E_TYPE_MISMATCH`; an unknown hash is `E_NOT_FOUND`; a group is `E_TYPE_MISMATCH` (SDD-021 section 2).
- **Order.** The table is applied after the colour matrix and the tone table, and before the filters (SDD-013 section 3.3 step 5). The senior records the exact place of the tone table in SDD-013 section 3.11 if it is not fixed there.
- **Plan.** The plan layer carries `lut` as `{asset, intensity}`, with the intensity a fraction rounded to three decimals (SDD-013 section 2.2). The export uploads the parsed table with `vr_put_lut` before the layer is drawn (SDD-004 section 3.6). A missing table is `missing_texture`, never a layer drawn without it.
- **Parser.** `engine/lut` is the only parser: 16 MiB, `LUT_3D_SIZE` from 2 to 65, no 1D table, and the refusals of SDD-004 section 3.6.
- **Parity.** The same table at the same intensity on an image layer gives the same bytes as on a clip of the same size (the golden cases `lut-warm` and `lut-teal-half` of SDD-004 section 3.6).

## Tasks

- [ ] 1. Amend SDD-021 (the `assets` kind, the `layer.adjust` key `lut`, the plan, the refusals; section 14 decision 9 is resolved), SDD-013 (section 2.2 for an image layer, section 3.11 for the place of the table) and SDD-004 section 3.6 (images use the same table). Verify: `task lint:md` exits 0.
- [ ] 2. Implement the key in `core/image` with its validation and a golden case for each refusal. Verify: `go test ./core/image/... -run TestAdjustLUT` passes; then `task codegen` and `task generate:check` exit 0.
- [ ] 3. Plan and export: the plan field, the upload to the module, the draw. Verify: `go test ./engine/export/... -run TestImageLUT` passes against its golden PNG files.
- [ ] 4. Service: resolve the asset by its hash and check its kind for the document. Verify: `go test ./service/... -run TestImageLUT` passes.
- [ ] 5. Parity with the video: the same table gives the same bytes on an image layer and on a clip. Verify: `go test ./engine/export/... -run TestImageLUTMatchesVideoLUT` passes.
- [ ] 6. Inspector control and both languages. Verify: `bun run check:i18n` and `task test:web` pass, and `web/e2e/specs/image-lut.spec.ts` (new, `bun run e2e`) passes.
- [ ] 7. Confirm the module: the Rust gate and the pinned digest. Verify: `cargo fmt --check && cargo clippy -- -D warnings && cargo test && cargo deny check` in `renderer/` exit 0, and `bash scripts/build-renderer.sh` reproduces the pinned digest.

## Test strategy

- Unit: the validation of the key; the stored form (intensity 0 is absent); each refusal.
- Golden: cases under `core/image/testdata/layer.adjust/`; a PNG golden of an image with a 17-point table at 100 % and at 50 %.
- Fuzz: the parser is the one of SDD-007 section 12 (`FuzzCubeParse`), unchanged; run it for 30 seconds: `go test -run=^$ -fuzz=FuzzCubeParse -fuzztime=30s ./engine/lut`.
- Parity: an image layer and a clip with the same table and size give equal bytes.
- End to end: set a table in the inspector; the picture shows the server's render with the table; export.

## Acceptance criteria

1. `layer.adjust` with `lut` at 100 % on an image layer changes the exported PNG to the table's values: `go test ./engine/export/... -run TestImageLUT` passes against its golden files.
2. An intensity of 0 leaves no key, and the export equals the export without the key: the same test passes.
3. A 1D table, a table over the limits of SDD-004 section 3.6, or a file that is not a `.cube` is refused with the same error as for a video: `go test ./engine/lut/...` passes, and `FuzzCubeParse` runs 30 seconds with no crash.
4. The same table gives the same bytes on an image layer and on a clip: `go test ./engine/export/... -run TestImageLUTMatchesVideoLUT` passes.
5. The three surfaces reach it through `layer.adjust`: `go test ./web/contract/...` passes, and `go run ./internal/tools/surfacebudget` exits 0.
6. `go test ./core/... ./engine/export/... ./service/... ./web/contract/...`, `task generate:check`, `bun run check:i18n`, `task test:web` and the Rust gate pass.

## Risks and mitigation

- **"Adjustment layer" as a new layer kind.** The wording of the request could mean a separate layer. A new kind would change SDD-021 section 2 and the plan, and SDD-021 section 5 says the renderer needs no new concept. **STOP** and ask the orchestrator; this epic implements the per-layer key.
- **A `.cube` without a project.** If epic 48 is not merged and the store has no route for a table without a project, **STOP** and wait. No workaround that changes the asset model (SDD-005 section 3.4).
- **Project-scoped LUT assets.** A video's `lut` is an asset id of its project (SDD-001 section 3.5); an image uses the content hash. That choice is recorded in SDD-021 in task 1; SDD-005 does not change.
- **Two parsers or two arithmetics.** The table is parsed once by `engine/lut` and drawn by the module (SDD-004 section 3.6). A second parser or a float path is rejected in review.

## Size

S

## Hand-off for review

The diff of `epic/49-image-lut`; the golden PNG files; the fuzz output; the outputs of the acceptance commands; the Rust gate result.
