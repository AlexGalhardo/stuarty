# SDD-021: Image documents

- Status: Draft (2026-11-19). Written by epic 27 before its code.
- Depends on: ADR-0038 (an image is a second kind of document on the same core and the same renderer), ADR-0002, ADR-0005, ADR-0029, ADR-0031; SDD-000 (names), SDD-002 (the form of a command catalogue), SDD-003 (the surfaces), SDD-005 (persistence), SDD-013 (the plan the renderer reads)
- Implements: PRD-004 RF-330, RF-331, RF-332, RF-336, RF-337, RF-338, RF-339, NFR-061, NFR-062; author decision D-062
- Owner area: epic 27 (`core/image/`, `engine/export/image*.go`, `service/image*.go`, the `image` adapters of `web/app/`)

## Contents

1. Overview
2. The document
3. The catalogue
4. The batch machinery
5. The plan
6. Export
7. Templates
8. Lint
9. The text view
10. Storage
11. The service and the surfaces
12. Errors and limits
13. Tests
14. Decisions of this SDD

## 1. Overview

An image document is a canvas with an ordered tree of layers: image, text, shape, solid fill and group. It is edited by the same kind of command that edits a timeline (a named operation with a JSON Schema, applied in an atomic batch with a base revision, an idempotency key, a dry run, a log, undo and redo per actor, and conflict detection per object) and it is drawn by the same renderer: `core/image` builds a one-frame plan in the format of SDD-013 section 2 and the Go host draws it on the bit-exact CPU path.

What this SDD does not change: the video document, its catalogue, its plan and its renderer. An image operation lives in its own registry (domain `image`) so that a video batch can never reach it and the video tests keep their meaning. What it shares with the video core is code, not copies: the argument decoder and the JSON Schema generator of `core/command` (the tags of SDD-002 section A.2), the `Operation`, `Error`, `Primitive`, `Change` and `Guard` types, `timeline.TextStyle` and its validation for the style of a text layer, `timeline.Color`, the shape names, `plan.FramePlan`, `plan.BitmapSpec` and `lint.Finding`. A new rule that a video and an image both need goes to the shared package, never into a second copy.

Out of scope (epics 29 and 30): adjustments, filters, masks and the wider set of blend modes, operations with a model, brushes, selections by pixel, vector pen paths, RAW, CMYK and PSD import. The screen is epic 28.

## 2. The document

```json
{
  "schema_version": 1,
  "id": "img_3c5v8n2w6d",
  "name": "Episode 12 thumbnail",
  "width": 1280, "height": 720,
  "background": "#101820",
  "template": false,
  "assets": {"sha256:9f86...": {"hash": "sha256:9f86...", "name": "face.png", "width": 1600, "height": 900}},
  "layers": [
    {"id": "lyr_a1b2c3d4e5", "kind": "image", "name": "Photo", "visible": true, "opacity": 100,
     "x": 0, "y": 0, "w": 1280, "h": 720, "asset": "sha256:9f86...", "crop": {"x": 0, "y": 0, "w": 1600, "h": 900}},
    {"id": "lyr_f6g7h8j9k0", "kind": "text", "name": "Title", "visible": true, "opacity": 100,
     "x": 80, "y": 440, "w": 800, "h": 200, "text": "HOW IT WORKS", "slot": "title",
     "style": {"font_family": "Anton", "size": 120, "color": "#FFE600"}}
  ]
}
```

- **Canvas**: `width` and `height` in whole pixels, 1 to 4096 (the largest frame the renderer draws, SDD-013 section 7). `background` is `#RRGGBB`, `#RRGGBBAA` or `transparent` (a PNG or a WebP then keeps the alpha; a JPEG is flattened on white).
- **Coordinates are integers.** `x`, `y`, `w`, `h` are whole pixels of the parent (the canvas for a top-level layer, the group origin for a child); `rotation` is whole degrees clockwise, -360 to 360 (a layer turns around its centre); `opacity` is a whole percent, 0 to 100. No float is stored, so no float decides a pixel. The plan converts with a fixed rule (section 5).
- **Layers** have stable ids `lyr_` plus ten characters (SDD-000 section 2), unique in the document. The order of `layers` and of `children` is the stacking order, the first is the bottom. Every layer has `kind`, `name` (at most 120 characters), `visible`, `locked` (omitted when false), `opacity`, `x`, `y`, `w`, `h`, `rotation` and `slot` (optional).
  - `image`: `asset` (a content hash `sha256:...` of an image in the workspace store, which must also be a key of `assets`) and `crop` (`x`, `y`, `w`, `h` in pixels of the source; absent means the whole source). The crop must lie inside the source.
  - `text`: `text` (at most 2000 characters; an empty text is an empty slot when the layer has a `slot`) and `style` (a `timeline.TextStyle`: the same keys, bounds and validation as a text item of a video; `$name` colour references are not allowed). `w` is the width the text wraps at (`style.max_width` is not stored: the plan sets it to `w`) and `h` is the height of the box the text sits in; `align` of the style places the text block horizontally in the box and the block is at the top of the box.
  - `shape`: `shape` (`rect`, `rounded_rect` or `ellipse`), `fill`, `stroke_color` and `stroke_width` (whole pixels), `radius` (whole pixels, `rounded_rect` only).
  - `fill`: `fill` (a colour). It is a rectangle, created over the whole canvas, that a person can move and resize like any layer.
  - `group`: `children`. A group has no size of its own: its `x` and `y` are an offset added to every descendant, `w` and `h` are 0, `rotation` is 0, and its `opacity` multiplies the opacity of each descendant (isolated group compositing arrives with the blend modes of epic 29). Groups nest up to 8 levels.
- `assets` is the table of the images the document uses, by hash: `hash`, `name` (the base name, untrusted text), `width` and `height` of the source. It is filled by `layer.add_image` from what the service knows of the file, so the core never reads a file, and an entry no layer uses is removed by the same batch that removes the last use.
- A **template** (`"template": true`) is a document that names slots (section 7). Nothing else differs.

## 3. The catalogue

Eighteen operations in the registry of domain `image`. The names are in SDD-000 section 1. `text.set` and `shape.set` exist for video with another schema and other code (they take an `item_id`), so the image operations that edit a text or a shape are `layer.set_text` and `layer.set_shape`; the other names did not exist before. Every argument that names a layer is a `lyr_` id; an id that is not in the document is `E_NOT_FOUND`. A locked layer (or one inside a locked group) accepts only `layer.set` (to unlock it, rename it or change its slot) and the commands that read it as a target (`layer.align` to it); any other change is `E_INVALID_STATE`. Every command that adds a layer takes an optional `id` (the caller's id, `E_ID_EXISTS` when taken), an optional `parent` (a group; absent means the top level) and an optional `index` (the position among its siblings, 0 is the bottom; absent means on top) and returns `effective.layer_id`.

| Operation | Arguments (`?` optional) | What it does | Errors |
| --- | --- | --- | --- |
| `image.create` | `preset` or `width` and `height`, `name?`, `background?`, `template?` | Sets the canvas, name, background and template flag of a new document; refused on a document that already has a canvas | `E_VALIDATION`, `E_INVALID_STATE` |
| `canvas.resize` | `width`, `height`, `mode?` (`keep` default, or `scale`) | Changes the canvas; `keep` leaves every layer where it is, `scale` multiplies `x`, `y`, `w`, `h` of every layer by the ratio of the new size to the old, rounded half up (text sizes and stroke widths are not scaled) | `E_VALIDATION` |
| `layer.add_image` | `asset`, `name?`, `x?`, `y?`, `w?`, `h?`, `fit?` (`cover` or `contain`, default `contain`), `slot?`, `id?`, `parent?`, `index?` | Adds an image layer; without a box it is the `fit` of the source on the canvas, centred | `E_VALIDATION`, `E_NOT_FOUND`, `E_ID_EXISTS`, `E_LIMIT` |
| `layer.add_text` | `text`, `name?`, `x?`, `y?`, `w?`, `style?`, `slot?`, `id?`, `parent?`, `index?` | Adds a text layer; `w` defaults to 86% of the canvas, `x` to centre that width, `y` to the vertical centre of the text box; `h` is measured | `E_VALIDATION`, `E_NOT_FOUND`, `E_ID_EXISTS`, `E_LIMIT` |
| `layer.add_shape` | `shape`, `w`, `h`, `x?`, `y?`, `fill?`, `stroke_color?`, `stroke_width?`, `radius?`, `name?`, `id?`, `parent?`, `index?` | Adds a shape; `x` and `y` default to centre it on the canvas, `fill` to `#FFFFFF` | `E_VALIDATION`, `E_NOT_FOUND`, `E_ID_EXISTS`, `E_LIMIT` |
| `layer.add_fill` | `color`, `name?`, `id?`, `parent?`, `index?` | Adds a fill over the whole canvas | `E_VALIDATION`, `E_NOT_FOUND`, `E_ID_EXISTS`, `E_LIMIT` |
| `layer.group` | `layer_ids` (1 to 100), `name?`, `id?` | Wraps layers that share one parent in a new group at the place of the topmost of them, keeping their relative order | `E_VALIDATION`, `E_NOT_FOUND`, `E_INVALID_STATE`, `E_ID_EXISTS`, `E_LIMIT` |
| `layer.ungroup` | `layer_id` | Replaces a group by its children at its place, adding its offset to their `x` and `y` and folding its opacity into theirs | `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE` |
| `layer.move` | `layer_id`, `parent?` (a group id, or `root` for the top level), `index?` (the final position among the new siblings) | Changes the order and the parent; a layer cannot be moved into itself or a descendant | `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE`, `E_VALIDATION`, `E_LIMIT` |
| `layer.transform` | `layer_id`, any of `x`, `y`, `w`, `h`, `rotation` | Sets the box and the rotation; a group takes only `x` and `y` | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE` |
| `layer.set` | `layer_id`, any of `name`, `opacity`, `visible`, `locked`, `slot` (an empty string removes it) | Sets the common properties; a slot goes only on a text or image layer, and its name is unique | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_ID_EXISTS` |
| `layer.crop` | `layer_id`, `x`, `y`, `w`, `h` or `reset` | Sets the source rectangle of an image layer; the box keeps its size | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_OUT_OF_SOURCE`, `E_INVALID_STATE` |
| `layer.duplicate` | `layer_id`, `id?`, `name?`, `dx?`, `dy?` | Copies a layer with its whole subtree and new ids, just above the original, offset by `dx` and `dy` (default 0) | `E_NOT_FOUND`, `E_ID_EXISTS`, `E_LIMIT` |
| `layer.remove` | `layer_id` | Removes a layer and its subtree; undo puts it back | `E_NOT_FOUND`, `E_INVALID_STATE` |
| `layer.set_text` | `layer_id`, `text?`, `style?` (merged key by key, `null` removes a key), `reset_style?` | Changes the content or the style of a text layer and re-measures its height | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE`, `E_FONT_UNKNOWN` |
| `layer.set_shape` | `layer_id`, any of `shape`, `fill`, `stroke_color` (an empty string removes it), `stroke_width`, `radius` | Changes a shape or the colour of a fill layer (`fill` only); the radius goes with `rounded_rect` and leaves with another shape | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE` |
| `layer.align` | `layer_ids`, `to` (`canvas` or a layer id), `horizontal?` (`left`, `center`, `right`), `vertical?` (`top`, `middle`, `bottom`) | Moves each layer so its box edge or centre meets the edge or centre of the target; the box of a group is the union of its children; at least one of the two axes | `E_VALIDATION`, `E_NOT_FOUND`, `E_INVALID_STATE` |
| `layer.fit` | `layer_id`, `mode` (`cover` or `contain`), `to?` (`canvas` default, or a layer id) | Scales the box of an image, a shape or a fill, keeping its aspect (the crop of an image layer, the box of the others), to cover or to be contained in the target box, and centres it on it; a text layer and a group are sized with `layer.transform` | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE` |

Each operation has its JSON Schema, generated from the arguments type with the tags of SDD-002 section A.2, an example and a list of error codes; `task codegen` writes them to `core/schema/image/commands/<name>.schema.json` and the snapshot `core/schema/image/commands.json`. The golden cases (`core/image/testdata/<operation>/<case>/`) hold, per operation, one case that succeeds with its expected document, diff, inverse and effective values, and one for each error code it can return, in the form of the golden cases of the video catalogue. A test fails when an operation has no case.

## 4. The batch machinery

`image.ApplyBatch(doc, ops, env)` runs the operations in order on one private copy; an error fails the whole batch with `op_index`, leaves the input untouched and returns nothing. It is pure: `Env` carries the id generator, the lookup of an asset (`Asset(hash) (AssetInfo, bool)`) and the text measurer (`plan.Measurer`), all supplied by the caller, and the same document, operations and environment always give the same result.

- **Diff and inverse** are computed from the flat form of a document: the document fields (`name`, `width`, `height`, `background`, `template`) as one object `document`, and every layer as one object in the collection `layers` with its own fields plus `parent` (the id of its group, `""` at the top level) and `order` (its index among its siblings), the assets as the collection `assets`. The diff is the net change by object (`layer.added`, `layer.updated` with `changes` of `[before, after]` pairs, `layer.removed`, `asset.added`, `asset.removed`, `document.updated`); the inverse is the primitives `patch`, `put` and `remove` of SDD-002 section A.6 over the same collections, in the same shape as the video ones. Rebuilding the tree from the flat objects, sorted by parent and order, is the one place where the order is decided.
- **Undo and redo** apply a stored inverse with `image.ApplyPrimitives(doc, primitives, guard)`: a primitive whose object was changed by another actor since (the `Guard`) is skipped and reported with its reason (`field_changed_by_other_actor`, `object_changed_by_other_actor`, `object_missing`, `id_exists`), exactly as SDD-005 section 12.2; a result that would be invalid skips the primitives of the layers it names. The bookkeeping (the actor's own steps, redo closed by a new edit, a step whose primitives are all skipped committed with an empty diff and `applied` false so that it is consumed) is that of SDD-005 section 12, on the tables of section 10.
- **Conflicts** (SDD-002 section A.7): `image.Conflicts(ops, intervening)` reports the first layer named by the operations that an intervening change added, updated or removed; the unit is the object. A new-layer position (`parent`, `index`) names its parent group, and the canvas operations name `document`.
- **Idempotency** and **dry run** are the service's, as for video (SDD-005 section 10): the same key with the same operations returns the stored result, a dry run returns the diff and the description and writes nothing.
- A `NoOp` batch (an empty diff) commits nothing, as in VV-050.

## 5. The plan

`image.Plan(doc, opts) plan.FramePlan` (opts: the measurer and the output scale) returns the one frame of the image in the format of SDD-013 section 2, with no new concept for the renderer:

- `Frame` 0, `Width` and `Height` the canvas, `Background` the colour (`#000000` and `Transparent` true for `transparent`; an alpha in `#RRGGBBAA` is ignored as in video), `Layers` bottom first.
- The tree is flattened depth first, bottom first. A hidden layer or group, and its subtree, makes no plan layer. The group offsets are added to the coordinates and the opacities multiplied; `opacity` of a plan layer is the percent divided by 100, rounded to three decimals.
- Every plan layer has `item` = the layer id, `cx` and `cy` = the centre of the box (`x + w/2`, `y + h/2`), `w` and `h` the box, `rotation` degrees clockwise, `scale` 1.
- `image` becomes a `media` layer with `asset` = the content hash, `asset_kind` `image` and `crop` the source rectangle (the whole source when the layer has no crop).
- `shape` and `fill` become `bitmap` layers with a `shape` bitmap (`rect` for a fill).
- `text` becomes a `bitmap` layer with a `text` bitmap, whose style is the resolved style of the layer (the defaults of a video text item, `max_width` = `w`). The measurer gives the natural size of the block, `fit` shrinks a block with a word wider than `w` as in SDD-004 section 5.1, and the block is placed at the top of the box and, by the style's `align`, at its left, centre or right.
- No effect, mask, blend, shadow or outline is set: those are epic 29.

The plan is the same on every machine: it holds numbers rounded to three decimals and the text measurer is the one of `engine/rasterize` (the Rust renderer, one module for every host).

## 6. Export

`engine/export` renders the plan with the Rust renderer through its Go host on the CPU path (`engine/renderhost`, the bit-exact one; ADR-0003, ADR-0029): it rasterizes each bitmap (`engine/rasterize`), decodes each image layer from the normalized PNG of its asset (the same artifact a video clip uses), uploads them as the video export does and takes the pixels. `ImageExport` then encodes them:

| Format | Encoder | Notes |
| --- | --- | --- |
| `png` | Go standard library `image/png`, best compression | Straight alpha; deterministic |
| `jpeg` | Go standard library `image/jpeg`, `quality` 1 to 100 (default 90) | Flattened on white; 4:2:0 |
| `webp` | the `WebPEncoder` of `engine/export` (`service.Options.WebP`), lossless (default) or `quality` 1 to 100 | **Not wired yet**: the export fails with `E_EXPORT_FAILED`, `details.reason` `webp_unavailable`, and PNG and JPEG still work. The intended encoder is the FFmpeg `libwebp` the product already runs as a separate process (ADR-0003), but the argument grammar of `engine/ffmpeg/ffargs` (SDD-007 section 6) has no builder that encodes a still image, and adding one is a change of that security check that waits for the author |

No new dependency: PNG and JPEG are the standard library, and WebP waits for the FFmpeg builder above. The export is deterministic (NFR-062): the same document, the same assets and the same options give the same bytes; a test renders twice and compares the files, and another compares the PNG with golden files. `width` and `height` of the request resample the result to another size (the document is always drawn at its canvas size at scale 1, and the output is resized with a fixed integer filter), and `preset` does the same for the sizes below. The file goes to `exports/images/<image id>/<revision>-<sha256 first 12>.<ext>` of the workspace, written atomically, and the result gives the path, the size in bytes, the digest and the pixel size.

Presets (data in `core/image/presets.go`): `video_thumbnail` 1280 x 720, `square_post` 1080 x 1080, `vertical_cover` 1080 x 1920. A preset names a size for a new document (`image.create`) and a minimum text size for the lint (section 8): 36 px for the thumbnail, 32 px for the square post and 40 px for the vertical cover; a document with no preset uses 24 px.

## 7. Templates

A template is a document with `template: true` whose text and image layers may carry a `slot` (`^[a-z][a-z0-9_]{0,39}$`, unique in the document). `image.FromTemplate(template, values, env)` validates the values and returns a new document (a new id, `template` false, the layers kept with their ids):

- `values` maps a slot name to `{"text": "..."}` for a text slot or `{"asset": "sha256:..."}` for an image slot. A name that is no slot of the template, a value of the wrong kind or an invalid value (a text too long, an asset that is not an image) is `E_SLOT` with `details.reason` `unknown_slot`, `wrong_kind` or `invalid_value` and `details.slot`.
- A slot with no content in the template (an empty text, an image layer that has no asset) is **required**; a slot that already has content is optional and keeps it. A required slot with no value is `E_SLOT` with `details.reason` `missing` and `details.missing` listing every slot name that lacks one, and nothing is created.
- An image slot keeps its box and gets the asset with the crop that covers the box (`fit` `cover`, centred); a text slot keeps its box and style and is re-measured.

`ImageCreate` takes `template_id` and `values`. A document with a slot that still has no content is not exportable: the lint says so (section 8) and `ImageExport` refuses it with `E_LINT_BLOCKING`.

## 8. Lint

`image.Lint(doc, env)` returns `lint.Finding` values (`item` is the layer id; no frame range), sorted by severity, then layer order, then code.

| Code | Severity | Blocks an export | Fires when |
| --- | --- | --- | --- |
| `L_IMAGE_EMPTY_SLOT` | error | yes | a slot layer has no content: an empty text or an image layer with no asset |
| `L_IMAGE_TEXT_OUTSIDE` | warning | no | the box of a visible text layer (rotation applied) is not entirely inside the canvas |
| `L_IMAGE_TEXT_SMALL` | warning | no | a text layer's `size` is below the minimum of the preset of the document (24 px with none) |
| `L_IMAGE_LOW_CONTRAST` | warning | no | the contrast ratio (WCAG relative luminance) between the text colour and the colour under the centre of its box is below 3, where "under" is the topmost visible fill or shape box that contains the centre and is below the text, else the canvas background; an image layer under the text is not sampled (the lint reads no pixel) and reports nothing |
| `L_IMAGE_UPSCALED` | warning | no | an image layer's box is more than 2 times larger, on either side, than the crop of its source (`w` over crop `w`, `h` over crop `h`, rotation ignored) |

The rules are pure functions of the document and of the text measurer. Their catalogue and `Blocking(code)` are those of `core/image` (`LintCatalog`, `Blocking`, which blocks `L_IMAGE_EMPTY_SLOT` only), not of `core/lint`, whose catalogue is the table of SDD-003 section 3.6 that the video lint documentation prints; the findings themselves are `lint.Finding` values. A message names a layer by its id, never by its name (SDD-007 section 8.2). A batch result carries the findings that the batch introduced (at most ten) and the totals, as for video (SDD-003 section 2.4 rule 2).

## 9. The text view

`ImageDescribe` returns the document as text an agent reads in one call, inside a budget of 4,000 characters (the cap of `describe` in the batch result, SDD-003 section 2.4 rule 6): a header line (`image img_... r12 "name" 1280x720 background #101820 template=false layers=5`), the layer tree, one line per layer, indented by depth, top layer first (the order a person sees):

```text
lyr_f6g7h8j9k0 text "Title" at 80,440 800x200 opacity 100 slot=title "HOW IT WORKS" size 120
lyr_a1b2c3d4e5 image "Photo" at 0,0 1280x720 opacity 100 asset sha256:9f86... crop 0,0 1600x900
```

then the findings of the lint, one line each, errors first. When the text would pass the budget the tree is cut at a line boundary and ends with `... and N more layers` and the findings keep their reserved share (at least 1,000 characters). The same text goes in `text` of the result; the structured `layers` and `findings` go next to it. Text of the document (`name`, layer names, text content) is untrusted and is marked as such like the other texts of SDD-007 section 8.2: every view that quotes one (the text view, the diff of a batch, the list, the edit, undo and redo texts) starts with the fixed `note:` line, counted in the budget, and quotes it in the compact form of the timeline diff (`command.Compact`: control and bidirectional characters removed, `"`, `\`, `<` and `&` escaped, a name cut at 60 characters and a text at 60).

## 10. Storage

An image document is stored in its own tables, `image_documents` and `image_operations`, created by one migration (SDD-005 section 5.13). The epic of projects (06) reserves the item kind `image` of a production; until it is merged an image has its own id (`img_`), and the item of a project will hold that id, so no row moves. The tables follow `projects` and `operations`: the document as canonical JSON with its digest, the revision, and an append-only log of one row per batch with its inverse, diff and skipped primitives, `closed_by` for undo and redo, and the idempotency key with its stored result. The writer is serialized by the one write transaction of the store. In network mode an image has an owner and is visible to its owner and to an administrator, as a project is (SDD-007 section 4.1).

**Access.** An image belongs to no project until the production exists, so a key limited to one project reaches no image: every image operation answers `E_FORBIDDEN` for it, `ImageCreate` included. In network mode the server picks the id of a new image (a request with an `id` is `E_VALIDATION`, so the answer never says that another user has an image with that id), a user sees only the images they own, and a file may be put in an image only when a project of the caller holds it, the rule of the asset library (`AssetList`): the check is made when a layer is added or a template is filled and again when the export reads the file, and a file the caller may not use is as unknown as one that does not exist, so its name is never returned.

An image layer names its file by content hash. Today a file enters the content store through the media import of any project (`MediaImport` gives its `hash`); the service checks that the hash is a ready image in the store before the layer is added, and reads its dimensions from there. Importing a file straight into an image arrives with the screen (epic 28) and the production (epic 06).

## 11. The service and the surfaces

Seven operations are added to `service.Service`, each with exactly one MCP action, one CLI command and one HTTP route (SDD-000 section 1; additions only, nothing of the frozen surface changes). They take the scopes of the video operations (`edit` to create and edit and to export, read to describe and list).

| Operation | Does |
| --- | --- |
| `ImageCreate` | creates a blank document (a preset or a size, a background, a name) or one from a template with `values`; returns the id, the revision and the text view |
| `ImageList` | lists the documents and templates, newest first, paged |
| `ImageDescribe` | the text view of section 9 and the structured tree, at the current revision |
| `ImageEdit` | a batch of image operations with `base_revision`, `idempotency_key` and `dry_run`; returns the revision, the diff, the effective values, the findings the batch introduced and a description |
| `ImageUndo`, `ImageRedo` | the actor's last step undone or done again, with the skipped primitives |
| `ImageExport` | renders and writes a file (section 6), after the lint; returns the path, bytes, digest and size |

`CommandHelp` takes an optional `domain` (`video` default, or `image`), so an agent learns the catalogue, the schemas and the examples of the image operations through the same call, and the same flag exists in the CLI (`command:help --domain image`) and the same parameter in the MCP tool and the HTTP query.

## 12. Errors and limits

No new code in the core: `E_VALIDATION`, `E_NOT_FOUND`, `E_ID_EXISTS`, `E_TYPE_MISMATCH`, `E_INVALID_STATE`, `E_OUT_OF_SOURCE`, `E_LIMIT`, `E_SLOT` and `E_REVISION_CONFLICT` keep their meaning. The service adds none. Limits (`E_LIMIT`, `details.limit`): 500 layers in a document, 8 levels of groups, 100 layers in one `layer.group` or `layer.align`, 64 distinct images, a canvas side of 4096, 2,000 characters in a text, 200 operations and 512 KiB in a batch (the video limits), and 25 slots in a template.

An export renders in the request, on the caller thread, bounded by the canvas (4096 per side) and by the size of the output (8192 per side, 33,177,600 pixels). A layer box may be larger than the canvas (16384 per side), so the pixels of the layers are bounded by the renderer instead: each layer is uploaded as soon as it is drawn, so the process holds one layer at a time, a bitmap holds at most 33,554,432 pixels (`MAX_PIXELS` of the rasterizer), and what the module holds is bounded by its memory (1 GiB, `out_of_memory` past it). It is not yet in the group of heavy operations of SDD-000 section 10 (which is a list of seven call sites that the OpenAPI document and the site pages repeat), so only the general rate limit of the caller applies; adding it is a change of that list and of its tests, left to the epic that adds the image jobs (epic 28 or 42).

## 13. Tests

- Golden cases per operation (section 3), a determinism test (same input, same result) and a property test that the inverse of every applied batch restores the document.
- `TestUndo`, `TestConflict`, `TestDryRun`, `TestTemplate`, `TestLint` in `core/image`; `TestImageRender` (golden PNG files) and `TestImageExportDeterministic` in `engine/export`; `TestImage` in `service`; the contract and parity tests of `web/contract` for the seven operations; the reference scenario of `scripts/smoke-e2e.sh` (a thumbnail from a template, by the CLI and by MCP).
- A review rule: an image rule written outside `core/image`, a copy of a type of the video core instead of sharing it, or a second pixel path is rejected.

## 14. Decisions of this SDD

1. **A separate registry, shared code.** The image operations are registered in their own registry so that the video registry, its tests and its surfaces do not change; the decoder, the schema generator and the types are shared.
2. **Integers and percents.** Position, size, rotation and opacity are integers; the only float is the plan's, rounded to three decimals.
3. **Own tables and id until epic 06.** Cheap to undo: the project item will reference the `img_` id.
4. **Layer names `layer.set_text` and `layer.set_shape`** because the video names `text.set` and `shape.set` take an item and have another schema.
5. **WebP through FFmpeg `libwebp`**, no new dependency, once `engine/ffmpeg/ffargs` has a builder for it; until then `webp_unavailable`. PNG and JPEG from the standard library.
6. **Group opacity multiplies the children's.** The approximation is stated; isolated groups come with epic 29.
7. **Text height is measured by the caller's measurer** and stored, so a replay of the log gives the same document.
