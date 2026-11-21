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

Epic 29 adds adjustments, filters, masks and blend modes to a layer (sections 2, 3.1 and 5). Out of scope (epic 30 and later): operations with a model, brushes, selections by pixel, vector pen paths, RAW, CMYK and PSD import. The screen is epic 28.

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
- **Look** (epic 29, every layer but a group; all optional, absent means none, and the stored form never holds a neutral value). `adjust` (section 3.1), `filters` (an ordered list, at most 8, each kind at most once), `mask` (a shape, or the alpha of another layer) and `blend` (a blend mode, absent means `normal`). A group has none of them: isolated group compositing (a group drawn on its own and then blended, adjusted or masked as one picture) needs a plan with sub-compositions that the renderer does not have, and stays a later epic; until then the group opacity is the multiplication stated above and `layer.adjust`, the filter and mask commands and `layer.blend` answer `E_TYPE_MISMATCH` for a group.
- `assets` is the table of the files the document uses, by hash: `hash`, `name` (the base name, untrusted text), `kind` (`image` or `lut`; an entry without `kind` is an image, so a document written before epic 49 keeps its meaning), and, for an image, `width` and `height` of the source (a `lut` entry has neither). An image entry is filled by `layer.add_image`, and a `lut` entry by `layer.adjust` with `lut` (section 3.1), from what the service knows of the file, so the core never reads a file, and an entry no layer uses (as an image, or as the table of an `adjust`) is removed by the same batch that removes the last use. Images and tables share the limit of 64 entries, at most 32 of them tables (section 12); both are judged on the entries the batch leaves, so swapping the table of a layer at the limit is allowed.
- A **template** (`"template": true`) is a document that names slots (section 7). Nothing else differs.

## 3. The catalogue

Twenty-seven operations in the registry of domain `image`. The names are in SDD-000 section 1. `text.set` and `shape.set` exist for video with another schema and other code (they take an `item_id`), so the image operations that edit a text or a shape are `layer.set_text` and `layer.set_shape`; the other names did not exist before. Every argument that names a layer is a `lyr_` id; an id that is not in the document is `E_NOT_FOUND`. A locked layer (or one inside a locked group) accepts only `layer.set` (to unlock it, rename it or change its slot) and the commands that read it as a target (`layer.align` to it); any other change is `E_INVALID_STATE`. Every command that adds a layer takes an optional `id` (the caller's id, `E_ID_EXISTS` when taken), an optional `parent` (a group; absent means the top level) and an optional `index` (the position among its siblings, 0 is the bottom; absent means on top) and returns `effective.layer_id`.

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
| `layer.replace_source` | `layer_id`, `asset` | Gives an image layer another file (epic 30, section 3.2), keeping the layer: its id, name, slot, visibility, opacity, rotation, adjustments, filters, mask and blend, and its place | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE`, `E_LIMIT` |
| `layer.duplicate` | `layer_id`, `id?`, `name?`, `dx?`, `dy?` | Copies a layer with its whole subtree and new ids, just above the original, offset by `dx` and `dy` (default 0) | `E_NOT_FOUND`, `E_ID_EXISTS`, `E_LIMIT` |
| `layer.remove` | `layer_id` | Removes a layer and its subtree; undo puts it back | `E_NOT_FOUND`, `E_INVALID_STATE` |
| `layer.set_text` | `layer_id`, `text?`, `style?` (merged key by key, `null` removes a key), `reset_style?` | Changes the content or the style of a text layer and re-measures its height | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE`, `E_FONT_UNKNOWN` |
| `layer.set_shape` | `layer_id`, any of `shape`, `fill`, `stroke_color` (an empty string removes it), `stroke_width`, `radius` | Changes a shape or the colour of a fill layer (`fill` only); the radius goes with `rounded_rect` and leaves with another shape | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE` |
| `layer.align` | `layer_ids`, `to` (`canvas` or a layer id), `horizontal?` (`left`, `center`, `right`), `vertical?` (`top`, `middle`, `bottom`) | Moves each layer so its box edge or centre meets the edge or centre of the target; the box of a group is the union of its children; at least one of the two axes | `E_VALIDATION`, `E_NOT_FOUND`, `E_INVALID_STATE` |
| `layer.fit` | `layer_id`, `mode` (`cover` or `contain`), `to?` (`canvas` default, or a layer id) | Scales the box of an image, a shape or a fill, keeping its aspect (the crop of an image layer, the box of the others), to cover or to be contained in the target box, and centres it on it; a text layer and a group are sized with `layer.transform` | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE` |
| `layer.adjust` | `layer_id`, any of `brightness`, `contrast`, `saturation`, `temperature`, `tint`, `hue`, `exposure`, `levels` (`black?`, `white?`, `gamma?`), `curve` (a list of points; an empty list removes the curve), or `reset` | Sets the adjustments of a layer, merged key by key; a value that equals its neutral one is removed; `reset` removes every adjustment | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE` |
| `layer.filter_add` | `layer_id`, `kind`, the parameters of the kind (section 3.1), `index?` | Adds a filter to the list of a layer (on top, or at `index`), the parameters left out taking the defaults of the kind | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE`, `E_LIMIT` |
| `layer.filter_set` | `layer_id`, `kind`, any parameter of the kind | Changes the parameters of the filter of that kind, merged key by key | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE` |
| `layer.filter_remove` | `layer_id`, `kind` | Removes the filter of that kind | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE` |
| `layer.filter_move` | `layer_id`, `kind`, `index` (the final position in the list) | Changes the place of a filter: filters act in list order | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE` |
| `layer.mask_set` | `layer_id`, `kind` (`rect`, `rounded_rect`, `ellipse` or `layer`), `x?`, `y?`, `w?`, `h?`, `radius?`, `feather?`, `invert?`, `source?` | Sets the mask of a layer, replacing any mask it had (section 3.1) | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE` |
| `layer.mask_clear` | `layer_id` | Removes the mask of a layer | `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE` |
| `layer.blend` | `layer_id`, `mode` | Sets the blend mode of a layer (`normal` removes it) | `E_VALIDATION`, `E_NOT_FOUND`, `E_TYPE_MISMATCH`, `E_INVALID_STATE` |

Each operation has its JSON Schema, generated from the arguments type with the tags of SDD-002 section A.2, an example and a list of error codes; `task codegen` writes them to `core/schema/image/commands/<name>.schema.json` and the snapshot `core/schema/image/commands.json`. The golden cases (`core/image/testdata/<operation>/<case>/`) hold, per operation, one case that succeeds with its expected document, diff, inverse and effective values, and one for each error code it can return, in the form of the golden cases of the video catalogue. A test fails when an operation has no case.

### 3.1 Adjustments, filters, masks and blend modes

Added by epic 29 (PRD-004 RF-333, RF-334, RF-335). Every value is a whole number, so no float is stored; the plan turns them into the numbers of SDD-013 (section 5 here) and the arithmetic of the pixels is in SDD-013 section 3.11. A locked layer refuses these commands like any other change, and `layer.remove` clears the `mask` of every layer whose `source` was removed.

**Adjustments** (`layer.adjust`; a stored `adjust` holds only the keys that differ from neutral, `levels` and `curve` included):

| Key | Range | Neutral | Meaning |
| --- | --- | --- | --- |
| `brightness` | -100 to 100 | 0 | the brightness of SDD-004 section 3.5 times 100 |
| `contrast` | -100 to 100 | 0 | the contrast is `1 + contrast / 100` |
| `saturation` | -100 to 100 | 0 | the saturation is `1 + saturation / 100` |
| `temperature`, `tint` | -100 to 100 | 0 | the temperature and the tint of section 3.5 times 100 |
| `hue` | -180 to 180 | 0 | degrees |
| `exposure` | -300 to 300 | 0 | hundredths of a stop: the colour is multiplied by `2^(exposure / 100)` |
| `levels` | `black` 0 to 254, `white` 1 to 255 (above `black`), `gamma` 10 to 1000 | 0, 255, 100 | the black point and the white point map to 0 and 255; `gamma` is in hundredths, 100 leaves the middle where it is and a higher value brightens it |
| `curve` | 1 to 8 points `{x, y}` of 0 to 255, `x` strictly increasing | none | a tone curve through the points, straight between them, with `(0, 0)` and `(255, 255)` as the ends when the list does not give them |
| `lut` | `asset` (a content hash) and `intensity` (a whole percent, 1 to 100, default 100) | none | the colour lookup table (`.cube`) of the video core, drawn at `intensity` per cent (epic 49) |

Brightness, contrast, saturation, temperature, tint and hue are the colour matrix of the video core (`plan.ColorMatrix`); exposure, levels and the curve are one 256-entry table built by `core/image` (SDD-013 section 3.11.1).

**The lookup table** is a key of the look of the layer, as a clip has its `lut` (SDD-001 section 3.5), and not a layer of its own: an image has no adjustment layer, and the plan stays one frame of layers (section 5). `layer.adjust` takes `lut` as `{asset, intensity}`; the keys left out of `lut` keep their value, so `{"lut": {"intensity": 50}}` changes the strength of the table the layer has and is `E_VALIDATION` (`args.lut.asset`) for a layer that has none. `intensity` 0 removes the key, as `reset` does. The table is named by the content hash of a file of the workspace that the ingest kept as a `lut` asset (`MediaImport`, SDD-011 section 7.1: the same file a video uses, its normalized `.cube`); the image does not copy it, and the asset ids of a project are not used (SDD-001 section 3.5 names them for a clip; an image belongs to no project, section 10). The service resolves the hash for the document (`AssetInfo.Kind`), so the core never reads a file, and the entry is added to `assets` with `kind` `lut` and the name of the file. The refusals are those of `clip.set_color` for `lut_asset`: a hash that is not a ready file of the workspace is `E_NOT_FOUND` (`details.kind` `asset`), a file that is not a lookup table is `E_TYPE_MISMATCH`, and a group is `E_TYPE_MISMATCH` as for every adjustment. The file is read, with the limits of SDD-004 section 3.6, only by the export; a file that no longer parses fails the export (`E_EXPORT_FAILED`), never a layer drawn without its table.

**Filters** (`layer.filter_*`; `kind` is also the name of the filter inside the layer, so a layer has at most one of each kind, and at most 8 filters). A parameter left out takes the default of the kind:

| Kind | Parameters (range, default) | Drawn as |
| --- | --- | --- |
| `grayscale`, `sepia`, `invert`, `warm`, `cool` | `amount` (1 to 100, 100) | the colour filter of SDD-004 section 3.6 mixed in by `amount` per cent |
| `blur` | `radius` (1 to 100, 8) | the Gaussian blur of SDD-013 section 3.5, `radius` the standard deviation in canvas pixels |
| `sharpen` | `amount` (1 to 300, 100) per cent, `radius` (1 to 20, 2) | an unsharp mask on that blur (SDD-013 section 3.11.3) |
| `vignette` | `amount` (1 to 100, 50), `softness` (0 to 100, 50) | the vignette of SDD-004 section 3.13 |
| `grain` | `amount` (1 to 100, 30), `size` (1 to 8, 1), `seed` (0 to 65535, 0) | the grain of SDD-004 section 3.13 |
| `shadow` | `color` (`#000000`), `opacity` (0 to 100, 50), `dx` and `dy` (-200 to 200, 4), `blur` (0 to 100, 8) | the drop shadow of SDD-004 section 3.15 |
| `glow` | `color` (`#FFFFFF`), `radius` (1 to 100, 12), `amount` (1 to 400, 100) | the outer glow of SDD-013 section 3.11.3 |
| `stroke` | `color` (`#000000`), `width` (1 to 100, 4) | the outline of SDD-004 section 3.15 |

The filters act on the pixels of the layer in list order (a `shadow` before a `blur` is blurred with the picture), after the adjustments and before the mask. Adding a kind the layer already has is `E_INVALID_STATE` (use `layer.filter_set`); a kind the layer lacks in `filter_set`, `filter_remove` or `filter_move` is `E_NOT_FOUND`; the ninth filter is `E_LIMIT` (`details.limit` `filters_per_layer`). The lookup table (`.cube`) is not a filter: it is the `lut` key of `layer.adjust`, drawn before the filters (see above and section 5).

**Masks** (`layer.mask_set` replaces, `layer.mask_clear` removes). A shape mask (`rect`, `rounded_rect`, `ellipse`) is in thousandths of the box of the layer: `x`, `y`, `w`, `h` (0 to 1000, `w` and `h` at least 1, `x + w` and `y + h` at most 1000; the defaults are the whole box: 0, 0, 1000, 1000), `radius` (`rounded_rect` only, 0 to 16384 pixels, default 16, held to half the smaller side when drawn), `feather` (0 to 200 pixels, the soft edge of SDD-004 section 3.9, default 0) and `invert` (default false). A mask of kind `layer` takes `source`, the id of another layer, and `invert`: the layer is drawn only where the source layer has pixels, scaled by the alpha of the source. The source must be an existing layer other than the layer itself, not a group, and without a mask of kind `layer` of its own (so there is no chain and no cycle); it may be hidden, in which case it is not drawn and still gives its shape. `x`, `y`, `w`, `h`, `radius` and `feather` are not valid for kind `layer`, and `source` is valid for it only.

**Blend modes** (`layer.blend`): `normal`, `multiply`, `screen`, `overlay`, `soft_light`, `hard_light`, `darken`, `lighten`, `color_dodge`, `color_burn`, `difference`. The formulas are SDD-013 section 3.11.2.

### 3.2 Replacing the file of a layer (epic 30)

`layer.replace_source` puts the result of an operation with a model (SDD-016 section 6.5) into the layer it was made from, and is useful on its own to swap a photo and keep everything around it. The caller gives the layer and the content hash of a ready image of the workspace (`E_NOT_FOUND`, as `layer.add_image`, when it is not); the layer must be an image layer (`E_TYPE_MISMATCH`) and not locked (`E_INVALID_STATE`); a layer that has no file yet, an empty slot, is filled. The rules are all in `core/image`:

- The layer keeps every property it has except its file and its crop. Its slot stays, so a template slot filled this way is still the slot.
- **A file of the size of the old one** (the usual answer of an edit) leaves the box and the crop exactly as they were.
- **Any other size** removes the crop, since its rectangle was in pixels of another file, and decides the box: when the new file has the proportions of the box (whole-number comparison of `w * new_height` with `h * new_width`) the box is as it was; when it has others the box is the largest that fits inside the old box with the proportions of the new file, centred on the centre of the old box, rounded as `layer.fit` `contain` rounds. The layer is never stretched, so a photo whose background was removed, which the provider may return in a shape of its own, stays where the photo was. A layer with no file yet, an empty slot, has no old size and takes this rule.
- **The table of images** gets the new file, and the old file leaves it when no other layer uses it. The limit of 64 distinct images is judged on the table the document is left with (`E_LIMIT`).
- `effective` names `layer_id`, the new `box` and `asset`.

## 4. The batch machinery

`image.ApplyBatch(doc, ops, env)` runs the operations in order on one private copy; an error fails the whole batch with `op_index`, leaves the input untouched and returns nothing. It is pure: `Env` carries the id generator, the lookup of an asset (`Asset(hash) (AssetInfo, bool)`) and the text measurer (`plan.Measurer`), all supplied by the caller, and the same document, operations and environment always give the same result.

- **Diff and inverse** are computed from the flat form of a document: the document fields (`name`, `width`, `height`, `background`, `template`) as one object `document`, and every layer as one object in the collection `layers` with its own fields plus `parent` (the id of its group, `""` at the top level) and `order` (its index among its siblings), the assets as the collection `assets`. The diff is the net change by object (`layer.added`, `layer.updated` with `changes` of `[before, after]` pairs, `layer.removed`, `asset.added`, `asset.removed`, `document.updated`); the inverse is the primitives `patch`, `put` and `remove` of SDD-002 section A.6 over the same collections, in the same shape as the video ones. Rebuilding the tree from the flat objects, sorted by parent and order, is the one place where the order is decided.
- **Undo and redo** apply a stored inverse with `image.ApplyPrimitives(doc, primitives, guard)`: a primitive whose object was changed by another actor since (the `Guard`) is skipped and reported with its reason (`field_changed_by_other_actor`, `object_changed_by_other_actor`, `object_missing`, `id_exists`), exactly as SDD-005 section 12.2; a result that would be invalid skips the primitives of the layers it names. The bookkeeping (the actor's own steps, redo closed by a new edit, a step whose primitives are all skipped committed with an empty diff and `applied` false so that it is consumed) is that of SDD-005 section 12, on the tables of section 10.
- **Conflicts** (SDD-002 section A.7): `image.Conflicts(ops, intervening)` reports the first layer named by the operations that an intervening change added, updated or removed; the unit is the object. A new-layer position (`parent`, `index`) names its parent group, and the canvas operations name `document`.
- **Idempotency** and **dry run** are the service's, as for video (SDD-005 section 10): the same key with the same operations returns the stored result, a dry run returns the diff and the description and writes nothing.
- A `NoOp` batch (an empty diff) commits nothing, as in VV-050.
- **Revert, checkpoints and the diff of two revisions (epic 50, RF-120, RF-121).** An image has what a timeline has (SDD-003 section 2.5.1, SDD-005 sections 12.4 and 5.14), with the same code: the rule of the walk, the ceilings, the names and the automatic checkpoint are written once and used by both. `image.Rebuild(current, inverses)` gives the document before a run of revisions: the stored `inverse` of each row, newest first, applied with `ApplyPrimitives` and no guard to one working copy, validated once; a primitive that does not apply is an error, never a partial document. The service rebuilds the target revision by walking back from the current one, refuses a row that is not of kind `edit`, `undo` or `redo` with `E_INVALID_STATE`, a walk of more than 5,000 rows with `E_LIMIT`, and a result whose canonical SHA-256 is not the `doc_sha256` of the target row with `E_INTERNAL`. `ImageRevert` commits `image.Compare(current, rebuilt)` as one `edit` row whose `ops` is `[{"op": "image.revert", "args": {"revision": T}}]`, with its own `inverse` and `diff`, so `ImageUndo` of the same actor takes it back and the guard protects what another actor changed since; the base revision conflicts on the layers the revert changes (and on `document` when the canvas or the name changes). An empty diff writes nothing (`same`), a dry run writes nothing and returns the diff. `ImageDiff` compares two **rebuilt documents**, never the log, so it is right across undo and redo; `image.DiffEntries(a, b)` classifies each change (`added`, `removed`, `moved`, `changed`; `layer`, `asset` or `document`), most significant first, with a line an agent can act on (`added text "Title" lyr_... at 80,440 800x200`, `moved "Badge" lyr_...: x 0 -> 40`, `changed "Logo" lyr_...: opacity 100 -> 60`, `added asset sha256:... "photo.png" image 1600x900`; a layer that is still in the document is named by its quoted name and its id). The text of a diff is cut at 40 lines and 4,000 characters, as the description of an edit is, with a count of what was left out, and starts with the `note:` line of section 9, since a layer name and a text are quoted in it. The panel of the editor offers them (SDD-012 section 24).

- **The words of a history row (VV-279).** `ImageHistory` gives each entry a `description`, one line in English that says what the step did, built by the service with the same function as the history of a timeline: the distinct `summary` of the image commands the step ran, joined by a space; `Undid rN.` and `Redid rN.` for an undo and a redo (N is the revision the row acted on); `Reverted to rN.` for a row whose only operation is `image.revert`. It holds no layer text or name, and the editor shows it as received.

## 5. The plan

`image.Plan(doc, opts) plan.FramePlan` (opts: the measurer and the output scale) returns the one frame of the image in the format of SDD-013 section 2, with no new concept for the renderer:

- `Frame` 0, `Width` and `Height` the canvas, `Background` the colour (`#000000` and `Transparent` true for `transparent`; an alpha in `#RRGGBBAA` is ignored as in video), `Layers` bottom first.
- The tree is flattened depth first, bottom first. A hidden layer or group, and its subtree, makes no plan layer. The group offsets are added to the coordinates and the opacities multiplied; `opacity` of a plan layer is the percent divided by 100, rounded to three decimals.
- Every plan layer has `item` = the layer id, `cx` and `cy` = the centre of the box (`x + w/2`, `y + h/2`), `w` and `h` the box, `rotation` degrees clockwise, `scale` 1.
- `image` becomes a `media` layer with `asset` = the content hash, `asset_kind` `image` and `crop` the source rectangle (the whole source when the layer has no crop).
- `shape` and `fill` become `bitmap` layers with a `shape` bitmap (`rect` for a fill).
- `text` becomes a `bitmap` layer with a `text` bitmap, whose style is the resolved style of the layer (the defaults of a video text item, `max_width` = `w`). The measurer gives the natural size of the block, `fit` shrinks a block with a word wider than `w` as in SDD-004 section 5.1, and the block is placed at the top of the box and, by the style's `align`, at its left, centre or right.
- **Look** (epic 29), per layer that is not a group: `adjust` becomes `color_matrix` (the matrix of `plan.ColorMatrix`, from the six colour keys) and `tone` (the table of SDD-013 section 3.11.1, absent when exposure, levels and curve are neutral); `filters` becomes the `filters` list of SDD-013 section 3.11.3, one entry per filter in list order, with the document numbers divided as that section says (percentages to fractions) and the colour filters as `matrix` entries; a shape `mask` becomes `mask` (`rounded_rect` with its `radius`) and a `mask` of kind `layer` becomes `clip`, whose `source` is the item id of the plan layer of the source: it is drawn where it stands and, when it is hidden (itself or a group above it) or its opacity, its own times that of its groups, rounds to no weight (SDD-013 section 3.3 step 2.1), it is added as a `stencil` layer at its place with opacity 1 (drawn by nobody, read by the clip only); `blend` becomes `blend`; `adjust.lut` becomes `lut` (`{asset, intensity}`, `asset` the 64 hexadecimal digits of the content hash, without `sha256:`, because the renderer names a table by at most 64 bytes, and `intensity` the percent divided by 100, rounded to three decimals), drawn after the colour matrix and the tone table and before the filters (SDD-013 section 3.11). The export reads the table with `engine/lut`, the only parser, and uploads it with `vr_put_lut` before the layer is drawn (SDD-013 section 4.2); a table the module does not hold is `missing_texture`, never a layer drawn without it. A layer with none of them gets none of the fields, so a document without a look gives the plan it gave before epic 29.

The plan is the same on every machine: it holds numbers rounded to three decimals and the text measurer is the one of `engine/rasterize` (the Rust renderer, one module for every host).

## 6. Export

`engine/export` renders the plan with the Rust renderer through its Go host on the CPU path (`engine/renderhost`, the bit-exact one; ADR-0003, ADR-0029): it rasterizes each bitmap (`engine/rasterize`), decodes each image layer from the normalized PNG of its asset (the same artifact a video clip uses), uploads them as the video export does and takes the pixels. The render runs on the instance that does not test the context at every loop back edge (about three times faster, section 13.2); it checks the context between the layers, and when the context ends during the draw it gives up waiting at once and closes the instance when the draw, bounded by the limits of section 12, returns. `ImageExport` then encodes them:

| Format | Encoder | Notes |
| --- | --- | --- |
| `png` | `engine/export` `encodePNGParallel` (NFR-061, epic 55): rows filtered with Sub and deflated at the fastest level in pieces of 128 rows on every CPU, joined into one zlib stream | Straight alpha, RGB when nothing is transparent; lossless; deterministic, and the same bytes whatever the CPU count (the piece size is part of the format). A 4,000 by 4,000 picture encodes in about 40 ms where the standard encoder at its fastest level took half a second and at its best level fourteen seconds; the file is a little larger than the standard one, and any PNG decoder reads it |
| `jpeg` | Go standard library `image/jpeg`, `quality` 1 to 100 (default 90) | Flattened on white; 4:2:0 |
| `webp` | `FFmpegWebP` of `engine/export` (`service.Options.WebP`): FFmpeg with `libwebp`, a separate process (ADR-0003), through the builder `ffargs.WebPStill`; lossless (default) or `quality` 1 to 100 | Straight alpha kept; deterministic on one machine. The PNG of the picture goes to a run directory and the `.webp` comes back from it; a `.webp` past `STUARTY_MAX_EXPORT_BYTES` is refused with `size_limit` before it is read, and the run directory is removed on every path. A build with no `libwebp`, or no FFmpeg, fails with `E_EXPORT_FAILED`, `details.reason` `webp_unavailable`, and PNG and JPEG still work |

No new dependency: PNG and JPEG are the standard library, and WebP is the FFmpeg the product already runs. The export is deterministic (NFR-062): the same document, the same assets and the same options give the same bytes; a test renders twice and compares the files, and another compares the PNG with golden files. `width` and `height` of the request resample the result to another size (the document is always drawn at its canvas size at scale 1, and the output is resized with a fixed integer filter), and `preset` does the same for the sizes below. The file goes to `exports/images/<image id>/<revision>-<sha256 first 12>.<ext>` of the workspace, written atomically, and the result gives the path, the size in bytes, the digest and the pixel size.

**The export is a job (D-111).** `ImageExport` also records a job of the kind `image.export`, already `succeeded`, with no project (an image belongs to none, section 10) and no asset, and returns its id as `job_id`. The job keeps in its input the image id and, in network mode, the user who exported it (`owner_user_id`), and in its result (`image`) the format, revision, size and digest and the path of the file relative to the workspace, never an absolute one. It is read with `job_get` like any job (the absolute `path` only for a local caller). Who may use it is the user who exported it, while that user may still see the image (any local caller in local mode): another user, even for the export of a shared template every user may read, and a key limited to a project are refused. When it is used, the path is rebuilt from the image id, the revision, the digest and the format, never read from the job, and must lie in `exports/images/`. It adds no tool: `job_get` and `image` (`action` export) already exist. A publication takes the id as its `thumbnail` (SDD-020 section 4); the file is the one the job recorded, checked again against its size and digest when the post is made. The table `jobs` accepts the kind through the migration `20261120130000_jobs_image_export_kind` (SDD-005 section 5.6).

Presets (data in `core/image/presets.go`): `video_thumbnail` 1280 x 720, `square_post` 1080 x 1080, `vertical_cover` 1080 x 1920. A preset names a size for a new document (`image.create`) and a minimum text size for the lint (section 8): 36 px for the thumbnail, 32 px for the square post and 40 px for the vertical cover; a document with no preset uses 24 px.

### 6.1 The cache of composed pictures (epic 54)

A person edits one layer at a time, so drawing all of them again after every edit is the cost NFR-061 forbids. `export.ImageCache` lets `RenderImage` start from the picture of the layers that did not change:

- **What is cached.** Pictures of the canvas size, the premultiplied RGBA the renderer wrote. After a render the cache holds the picture of the layers below the top one (when that cut is allowed, below) and the whole picture.
- **The key.** The owner (`ImageConfig.CacheScope`, the user in network mode, empty in local mode) and a SHA-256 chain: the first link is the canvas (size, background, transparency) and each next link is the previous one and the JSON of the next plan layer, from the bottom. A plan layer names its asset by content hash, its looks, its mask, its blend mode and its geometry, so the same chain is the same pixels. There is no revision number to trust or to forget to bump. A render looks for the longest chain it holds; the whole document found is returned as it is (a copy, never the cached buffer), a shorter one is uploaded as one full-canvas opaque texture (not opaque on a transparent canvas) under the item `stuarty-prefix`, as the first layer of the plan, and only the layers above it are decoded, rasterized, uploaded and drawn. The draw of a layer is a source-over of integers onto the frame in plan order, and the copy of a full-canvas texture onto a frame is exact, so the result is the same pixels as the render of every layer (`TestImageCacheRenderIsTheFullRender`, `TestImageCacheStartsFromThePictureBelowTheTopLayer`, `TestImageCacheOnATransparentCanvas`; every golden file stays as it is).
- **Where a cut is not allowed.** A mask taken from another layer (`clip`) needs its source in the same draw: no cut falls between a layer and the layer it is clipped to (`TestImageCacheNeverCutsBetweenALayerAndItsClipSource`). A document that masks its photo with the top layer is drawn in one pass, and a custom font set (`ImageConfig.Fonts`) turns the cache off, because the fonts are not part of the key.
- **The bound.** 384 MiB of pixels by default (`DefaultImageCacheBytes`: six pictures of 4,000 by 4,000 pixels); the least recently used picture goes first, a picture larger than the whole limit is not kept, and the cache is safe for concurrent renders. Memory is the only cost: it holds nothing on disk.
- **The owner.** A picture is read only by the scope that wrote it (`TestImageCacheIsNotSharedAcrossUsers`, T-89): the same document of another user starts from nothing.
- **Who uses it.** A caller that draws the same document again and again (the picture the editor shows after each edit, `service.ImageFrame`) sets `ImageConfig.Cache` to one cache for the process and `CacheScope` to the owner of the key; an export leaves them unset, so a file is always drawn from the files of the workspace. `ImageFrame` uses one cache per service (`DefaultImageCacheBytes`) with the scope `Caller.OwnerUserID` (`TestImageFrameStartsFromTheCacheOfItsOwnUser`).

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

**Access.** An image belongs to no project until the production exists, so a key limited to one project reaches no image: every image operation answers `E_FORBIDDEN` for it, `ImageCreate` included. In network mode the server picks the id of a new image (a request with an `id` is `E_VALIDATION`, so the answer never says that another user has an image with that id), a user sees only the images they own, except the image templates that ship with the product (the recipes of SDD-018 section 17): those have no owner, are installed in every mode, and every user lists them (`image:list --templates only`), reads them and makes a document from them with `ImageCreate`, while a change to one is `E_FORBIDDEN` with the hint to make an own document from it (`service.SharedImageTemplate`: a template, no owner, made by the starter actor; in local mode there is one user and they stay editable). A file may be put in an image only when a project of the caller holds it, the rule of the asset library (`AssetList`): the check is made when a layer is added or a template is filled and again when the export reads the file, and a file the caller may not use is as unknown as one that does not exist, so its name is never returned.

An image layer names its file by content hash. A file enters the content store through the media import of any project (`MediaImport` gives its `hash`) or straight through `ImageImport` below; the service checks that the hash is a ready image in the store before the layer is added, and reads its dimensions from there.

**The item kind `image` of a production stays reserved.** `project_items.kind` accepts `brief`, `text`, `timeline` and `output` today (SDD-005 section 5.12), and holding an image document in a production is a change of that table and of the production that epic 48 does not make. `ImageImport` writes an image document with its own id and no project item, as `ImageCreate` does.

### 10.1 Importing a file into an image (epic 48)

`ImageImport` takes the file of `path` (an absolute path on the machine of the server, read with the path rules of `MediaImport`, SDD-003 section 2.6: the same roots in network mode, the same refusals `E_PATH_NOT_ALLOWED` and `E_FILE_NOT_FOUND`) or the library `hash` of a file already in the workspace, never both. A path enters the content store with the ingest of SDD-011 section 4.7 (the normalized PNG, the proxy and the thumbnail), its size limit (`E_LIMIT_EXCEEDED`, `import_bytes`) and its pixel limit (`import_pixels`) unchanged; the call waits for the ingest up to `wait_ms` (default 10,000, at most 25,000 as in `MediaImport`) and, when the file is not ready by then, answers `state: pending` with the `job_id` and the `hash` and creates nothing: the caller imports the `hash` again once the file is ready. A file whose ingest failed answers the error of the ingest; a ready file that is not an image (a video, an audio file, a `.cube` table) is `E_UNSUPPORTED_MEDIA` with `details.reason` `not_an_image`, and an unknown or unusable `hash` is `E_VALIDATION` with `path` `hash`.

- **Without `image_id`**, a new document. Its canvas is the size of the source when both sides are at most 4,096 pixels (`coreimage.MaxCanvas`); a larger source needs `width` and `height` (1 to 4,096 each) or a `preset`, and without them the answer is `E_LIMIT` (`details.limit` `canvas`) with the hint to give one. `width` and `height` or `preset` given for a smaller source win over the source. The name is `name`, or the name of the file without its extension (at most 120 characters, `Untitled` when empty). The document is made by the batch `image.create` followed by `layer.add_image` (fit `contain`, centred, the whole source) in one transaction that writes the row and the first log row, so a refusal at any step leaves no document: the rule that builds the batch is `coreimage.ImportBatch`, the service only runs it. The file stays in the store, as after any refused import, and is reused by the next one (it is the same content hash).
- **With `image_id`**, one `layer.add_image` through the machinery of `ImageEdit`, with `base_revision`, `idempotency_key` and `dry_run` as there; `name` is the name of the layer and `width`, `height` and `preset` are `E_VALIDATION`. It commits one revision and publishes `image.updated` (SDD-006 section 12).
- `idempotency_key` needs `image_id` (a new document has no revision to retry against; `E_VALIDATION` otherwise). `dry_run` writes nothing, not even the file: with a `hash` it answers the canvas and the layer the import would make.
- Scope `edit`. In network mode the owner rules of section 10 apply: the server picks the id, and a file the caller imported by `path` is usable in their images, held by their library row of that file (the rule of `AssetList` looks at the files a project holds, and an image import has no project), so another user's hash is as unknown as one that does not exist.

### 10.2 A file chosen in the browser (epic 48, task 7)

The browser has no path on the disk of the server, so a file chosen in the editor travels as the body of `POST /images/import/file` (the operation `ImageImportFile`, HTTP only, scope `edit`; the name avoids the word "upload" of SDD-007 section 10, which names what leaves the machine): the bytes of the file as the request body, and the members of `ImageImport` (`image_id`, `name`, `width`, `height`, `preset`, `wait_ms`, `base_revision`, `idempotency_key`, `dry_run`) as the query, with the name the browser gave the file in `file_name`. It is the same import as 10.1 from the moment the file is in the content store, with the limits below; a refusal creates no document. A file refused by its size leaves nothing in the store; a file the ingest refuses (not media, over the pixel limit) stays in the store as a failed file of the caller, as after any refused import of 10.1, and its bytes are never served.

The body is input from a web page (SDD-007 section 2, threats T-62 to T-66):

- **Size.** The router limit of 1 MiB is replaced, for this route only, by 64 MiB plus one byte (`MaxImageFileBytes`), or by `STUARTY_MAX_IMPORT_BYTES` when that is smaller. A declared `Content-Length` over the limit is `E_LIMIT_EXCEEDED` before a byte is read; a body with no length is read through the content store (`Put`), which reads at most the limit plus one byte, streams it to a temporary file and deletes the file when it is over, so memory is never proportional to the body. A larger picture is put on the server and imported by its path.
- **Name.** `file_name` is a label: only its base name is kept, normalized, with control, bidirectional and separator characters replaced and cut at 200 characters (SDD-007 section 7.1 step 14). The file is stored under its hash and nowhere else, and the name is shown as text from a file (`asset_name`, SDD-007 section 8).
- **Decoding.** No Go decoder reads the bytes. The ingest of SDD-011 section 4.7 probes them with the bundled LGPL FFmpeg in a child process under the memory, time and output limits of SDD-007 section 6.5, and refuses a picture over `import_pixels` (33,177,600 by default), over 8,192 pixels on a side, or whose size it cannot read, before the conversion that makes the normalized copy; a hostile file costs at most what that child process may use, never the memory of the server. What is not media is `E_UNSUPPORTED_MEDIA`, whatever the name or the declared type says, so HTML, SVG or a script is never an image and only the normalized PNG, proxy and thumbnail of a ready image are served.
- **Origin.** The request passes the checks of every route: the session or the token, and for a cookie session the `Origin` and `Sec-Fetch-Site` checks and a preflighted content type (SDD-007 section 3.6 steps 4 to 6). The screen therefore sends `Content-Type: application/json` with the raw bytes; the route binds no body, so the server never parses the bytes as JSON and ignores the declared type.

## 11. The service and the surfaces

Seven operations are added to `service.Service`, each with exactly one MCP action, one CLI command and one HTTP route (SDD-000 section 1; additions only, nothing of the frozen surface changes), and epic 48 adds the eighth, `ImageImport`, in the same way, and the HTTP-only `ImageImportFile` of section 10.2. They take the scopes of the video operations (`edit` to create, edit, export and import, read to describe and list).

| Operation | Does |
| --- | --- |
| `ImageCreate` | creates a blank document (a preset or a size, a background, a name) or one from a template with `values`; returns the id, the revision and the text view |
| `ImageList` | lists the documents and templates, newest first, paged |
| `ImageDescribe` | the text view of section 9 and the structured tree, at the current revision |
| `ImageEdit` | a batch of image operations with `base_revision`, `idempotency_key` and `dry_run`; returns the revision, the diff, the effective values, the findings the batch introduced and a description |
| `ImageUndo`, `ImageRedo` | the actor's last step undone or done again, with the skipped primitives |
| `ImageExport` | renders and writes a file (section 6), after the lint; returns the path, bytes, digest and size |
| `ImageImport` | an image file (a `path` on the server, or a library `hash`) becomes a new document with one image layer, or one more layer of `image_id` (section 10.1); returns the id, the revision, the layer and the text view |

Epic 50 adds six operations of the same kind, one MCP action (of `history`, with `domain` `image`), one CLI command and one HTTP route each (SDD-000 section 1): `ImageRevert`, `ImageDiff`, `ImageCheckpointCreate`, `ImageCheckpointList`, `ImageCheckpointRename` and `ImageCheckpointRemove`. They take the scopes of the timeline's (`read` for the diff and the list, `edit` for the others), the access of section 10 and the requests of SDD-003 section 2.5.1 with `image_id` in place of `project_id`; an image checkpoint lives in `image_checkpoints` (SDD-005 section 5.14.1) and `base_revision` is CLI and HTTP only.

`ImageModelEdit` (epic 30) is a ninth operation of another kind: it starts the job `image.edit` that removes the background of a layer, or generates an image into the document, with the provider of a connection, and, when the job is done, commits one batch of section 3 as the actor that asked (SDD-016 section 6.5). It is specified there because it spends money and sends a picture out; the document and its undo are as for any batch of this section.

`CommandHelp` takes an optional `domain` (`video` default, or `image`), so an agent learns the catalogue, the schemas and the examples of the image operations through the same call, and the same flag exists in the CLI (`command:help --domain image`) and the same parameter in the MCP tool and the HTTP query. An image operation asked by name with no `domain` is found too, since no other domain has its name (SDD-003 section 4.3).

## 12. Errors and limits

No new code in the core: `E_VALIDATION`, `E_NOT_FOUND`, `E_ID_EXISTS`, `E_TYPE_MISMATCH`, `E_INVALID_STATE`, `E_OUT_OF_SOURCE`, `E_LIMIT`, `E_SLOT` and `E_REVISION_CONFLICT` keep their meaning. The service adds none. Limits (`E_LIMIT`, `details.limit`): 500 layers in a document, 8 levels of groups, 100 layers in one `layer.group` or `layer.align`, 64 distinct files (images and lookup tables), 32 of them lookup tables (the tables the renderer holds at once, `render_luts` of SDD-013 section 7.2: the image is one frame, so a document with more could not be exported), a canvas side of 4096, 2,000 characters in a text, 200 operations and 512 KiB in a batch (the video limits), 25 slots in a template, 8 filters in a layer and 8 points in a curve.

### 6.2 Prepared layers and bands (epic 55)

Epic 54 left two costs: the looks of a layer (blur, sharpen, glow, shadow, outline, the colour steps) are about 90% of a cold composition, and an edit of the top layer still pays for an instance, for the upload of a 64 MB picture and for two copies of it. Both are the same arithmetic done more than once or one core at a time, so the render is split in two stages that the module already separates (SDD-013 section 3.3: steps 1 to 5 depend on the pixels and the looks of one layer, the last step on its opacity, position, mask and blend mode).

- **A prepared layer** is the picture of one layer after every look and before the draw: its pixels, the size of the rectangle that grew around them (a blur widens it by three radii on each side) and, for a bitmap, the offset of its centre. `vr_prepare` makes it on one instance, from a render input that holds that layer alone and at opacity 1; `vr_put_prepared` stores it on any instance as the texture of a media layer that is then drawn with no look (SDD-013 section 4.2.2). The preparation of a layer is the only place its looks are computed, so a prepared picture drawn is the same pixels as the layer drawn whole: the draw of a prepared layer is the draw of the layer, with the same sampling, mask, opacity and blend mode, on the same placement (`TestImageBandsEqualTheWholeRender` for every look and every blend mode; every golden file stays as it is).
- **Layers are prepared in parallel.** The layers of a render are prepared by a bounded set of workers, each on its own instance of the module (an instance costs about half a millisecond once the module is compiled), and the decoding of the photos and the rasterizing of the texts and shapes that feed them run on the same workers, so a cold export spends its looks on every core instead of one. The number of workers is the number of CPUs, never more than 8 and never so many that their pictures pass the memory limit of the module of section 6; the result does not depend on it.
- **The prepared picture is kept.** It is cached under the memory limit and the owner of section 6.1, with the key of the layer **without** its opacity, position, blend mode and mask: the JSON of the plan layer with `cx`, `cy`, `opacity`, `blend`, `mask`, `clip` and `stencil` taken out, which still holds the asset hash, the crop, the size, the rotation and every look, so the same key is the same prepared pixels. A change of opacity, position, blend mode or mask of a layer finds its prepared picture and prepares nothing again (`TestImageCacheKeepsThePreparedLayer`); a change of a look, of the size or of the source misses. A position is part of the key when it is not a multiple of half a pixel, because the grown rectangle is then not the same sum (a plan places a centre at `x + w / 2`, so it always is). The export, which has no cache, prepares every layer once.
- **The draw is in bands.** The prepared rectangle of a layer gives, exactly, the rows its draw can touch. The canvas is cut into horizontal bands, one per worker; each band is drawn on an instance that holds only the prepared pictures of the layers that reach it and writes only its own rows (`y0` and `y1` of the render input, SDD-013 section 2.1), and the rows are joined into the picture. Drawing a row does not depend on any other row, so the joined picture is the picture drawn whole (`TestImageBandsEqualTheWholeRender`). A document with a mask taken from another layer (`clip`) draws in one band, because that layer must be in the same draw (section 6.1).
- **An edit draws only what it touches.** A render that starts from a cached picture (section 6.1) copies it, finds the rows the layers above it reach from their prepared rectangles and draws only those rows: the instance of a band receives the rows of the cached picture it needs, not the whole canvas, and every other row of the result is the cached picture as it is. The preview of an edit of the top layer therefore costs its draw on the rows it covers, in parallel, and the copy of the picture, not an instance, a 64 MB upload and two copies of it.
- **A cached picture does not outlive the caller's access.** A render that uses a cache asks for the file of every image and lookup table of the layers it draws before it reads the cache, so an asset the caller lost access to is as missing as it is to an export (`TestImageCacheRefusesAnAssetTheCallerLostWhileItIsCached`); the lookup reads no file.
- **A cancelled frame is not wasted.** Preparing runs on its own goroutines and gives up waiting when the context ends; what a worker finishes is still put in the cache, so the next frame of a cold document finds it, and the instance is closed when the worker returns.
- **Limits.** The memory of one instance is bounded as in section 6 (1 GiB, `out_of_memory`), and a prepared picture is bounded by a texture of the module (8,192 pixels a side, `limit_exceeded`): a layer whose looks grow it past that is refused with `E_EXPORT_FAILED`, as before.

An export renders in the request (the workers of section 6.2 are joined, or abandoned as that section says, before it returns), bounded by the canvas (4096 per side) and by the size of the output (8192 per side, 33,177,600 pixels). A layer box may be larger than the canvas (16384 per side), so the pixels of the layers are bounded by the renderer instead: each layer is uploaded as soon as it is drawn, so the process holds one layer at a time, a bitmap holds at most 33,554,432 pixels (`MAX_PIXELS` of the rasterizer), and what the module holds is bounded by its memory (1 GiB, `out_of_memory` past it). It is not yet in the group of heavy operations of SDD-000 section 10 (which is a list of seven call sites that the OpenAPI document and the site pages repeat), so only the general rate limit of the caller applies; adding it is a change of that list and of its tests, left to the epic that adds the image jobs (epic 28 or 42).

## 13. Tests

- Golden cases per operation (section 3), a determinism test (same input, same result) and a property test that the inverse of every applied batch restores the document.
- `TestImageRevert*` and `TestImageDiff*` in `core/image` (the property: revert to revision R gives the canonical bytes of R, and revert then undo gives the document before the revert), `TestImageCheckpoint*`, `TestImageRevertThroughTheService` and `TestImageRevertDryRun` in `service` (epic 50);
- `TestUndo`, `TestConflict`, `TestDryRun`, `TestTemplate`, `TestLint` in `core/image`; `TestImageRender` (golden PNG files) and `TestImageExportDeterministic` in `engine/export`; `TestImage` in `service`; the contract and parity tests of `web/contract` for the seven operations; the reference scenario of `scripts/smoke-e2e.sh` (a thumbnail from a template, by the CLI and by MCP).
- A review rule: an image rule written outside `core/image`, a copy of a type of the video core instead of sharing it, or a second pixel path is rejected.

### 13.1 NFR-061 on the reference document (epic 29)

`engine/export` `TestImageReferenceDocument` builds the reference document of NFR-061 (4,000 by 4,000 pixels, 30 layers of 1,200 by 900 pixels or more, ten photos, ten rounded rectangles and ten ellipses, rotated) and renders it twice through the WebAssembly module under `wazero`, with a look on every layer (adjustments with a curve, sharpen, a shadow with a soft light blend, a glow with a rounded mask): `go test ./engine/export -run TestImageReferenceDocument -v` (`STUARTY_NFR061_PLAIN=1` leaves the looks out). It asserts that the two exports are the same bytes and prints the times. Until epic 54 it ran only when asked, because the two exports took more than two minutes; since epic 54 they take about half a minute, so it runs in every run of the package except the short one (the determinism of a look export is also held on every run by `TestImageLookRenderIsTheGoldenFileAndDeterministic`). Measured on the author's Windows machine **with other agents running** (so not a reference number): composed in about 51 s and exported (with the PNG) in about 75 s with the looks, 7 to 8 s and 10 to 11 s without them. A native release build of the CPU path draws one 1,200 by 900 layer with one filter in 16 to 143 ms (tone 7 ms over the plain draw, sharpen 66, shadow 87, glow 87, stroke 132, soft light 36, rounded mask 16), so the module under `wazero` is the cost, not the arithmetic. The budget of NFR-061 (2 s) was therefore **not met by the CPU path through `wazero`** when this paragraph was written; this paragraph records the CPU number so the gap is known, and section 13.2 records what epic 54 did about it.

### 13.2 Profile of the reference document before epic 54

Measured on the author's Windows machine with other agents running (so for the shares, not for the absolute numbers; the idle numbers are in `docs/perf/`). `BenchmarkImageReference`: composed 34.5 s, exported 49.1 s. One cold export of the reference document, stage by stage:

| Stage | Time | Share of the export |
| --- | --- | --- |
| Plan, rasterizing 20 shapes, decoding and scaling 10 photos, uploading them | about 2.4 s | 5% |
| `vr_render` in the module (the composition: draw, blend, blur, sharpen, shadow, glow, mask) | 30.5 s | 62% |
| PNG encode at `png.BestCompression` (64 MB of pixels, 3 MB of file) | 13.5 s | 27% |
| Module instantiation and the rest | under 1 s | 2% |

Inside the composition, three findings, each measured by running the module with one change at a time (a function listener of `wazero` for the shares):

- **The cancellation check costs a factor of three.** The host created for an image render was the cancellable one, which makes `wazero` test the context at every loop back edge; the same render on the uncancellable instance takes 11 s instead of 31 s (an empty 4,000 by 4,000 frame: 27 ms instead of 400 ms). The comment of `renderhost.Config.Uncancellable` already said "about five times faster".
- **The looks are 90% of the composition.** The same 30 layers with no look compose in 3.4 s; the looks add 27 s (sharpen with its adjustments on the ten photos 13 s, the shadow and soft light 7 s, the glow and the mask 8 s). A photo that is rotated is not scaled down before it is uploaded, so its sharpen blurs the 2,000 by 2,000 source.
- **Inside the module** the time is in `sample` and `mix_pixel` (about 45%: the bilinear draw of 41 million covered pixels), in the two box-blur passes (about 25%) and in the mask coverage (about 4%). Nothing in it is a CPU fallback or a repeated decode: each asset is decoded once and uploaded once.

The PNG encoder at its slowest level is the second cost and has nothing to do with the module.

What epic 54 changed, measured on the same loaded machine with `BenchmarkImageReference` (`go test -run '^$' -bench BenchmarkImageReference -benchtime 1x ./engine/export/`; `-run x` also runs every test with an x in its name and takes eight minutes):

| | Before | After |
| --- | --- | --- |
| Cold composition of the 30 layers | 34.5 s | 11 s |
| PNG encode | 13.5 s | 0.4 s |
| Cold export (`s_export`) | 49.1 s | 11.4 s |
| Composition after an edit of the top layer (`compose_ms`, cache of section 6.1) | 34.5 s (every edit drew everything) | about 0.5 s |

The edit that remains, 0.5 s, is the top layer itself: alone on a blank canvas it takes 0.36 s (its glow and its rounded mask, 0.1 s each, and the plain draw of 1.7 million pixels 0.07 s), and the rest is the instance, the upload of the picture below it and the two copies of 64 MB. The cold export is the sum of the looks. Both are the arithmetic of the module under `wazero` (a native release build draws one 1,200 by 900 layer with one filter in 16 to 143 ms), so the levers left are in `renderer/`: skipping the preparation of a layer (blur, sharpen, glow) when the band that is drawn does not touch it, which would let the export draw bands on several instances, and keeping the prepared image of a layer so that an edit of its opacity or position does not prepare it again. Those levers and the measurement against the budget on the idle machine are epic 55 (D-118).


### 13.3 After epic 55

Measured on the same Windows machine (16 logical CPUs, an AMD Ryzen 7 5700X3D) **with other agents running**, so the numbers are for the shares and the direction, never the budget: the idle numbers are the orchestrator's, in `docs/perf/`. `BenchmarkImageReference`, `-benchtime 5x`:

| | Before epic 55 | After |
| --- | --- | --- |
| Cold export (`s_export`), loaded machine | 16 s (11 s on a quieter one) | 3.5 to 5 s |
| Edit of the top layer (`compose_ms`), loaded machine | 440 ms | 50 to 70 ms |
| CPU time of one cold export | not measured | about 15 s over 8 workers: preparing 11 s, drawing 4 s, PNG 0.3 s |
| PNG encode | 0.5 s on one core | 40 ms |

What each step took out, in order: the layers prepared on 8 instances at once and drawn in bands (the looks are no longer one core's work); the instances kept in a pool (a fresh instance faulted in every page of its memory, which took longer than the draw once eight did it together); a band drawn into a buffer of its own rows; the prepared picture kept across an edit of an opacity or a position; the box blur on the channels that can be non-zero, by reciprocal and row by row (0.18 s to 0.08 s for a 2,000 by 2,000 picture, native); the draw reading four pixels instead of sixteen values (a rotated layer, 39 ms to 24 ms, native); the PNG compressed in pieces. A single rotated 2,000 by 2,000 photo with tone, colour matrix and sharpen still prepares in about 0.43 s in the module (0.17 s native), which is the longest task of the export.
## 14. Decisions of this SDD

1. **A separate registry, shared code.** The image operations are registered in their own registry so that the video registry, its tests and its surfaces do not change; the decoder, the schema generator and the types are shared.
2. **Integers and percents.** Position, size, rotation and opacity are integers; the only float is the plan's, rounded to three decimals.
3. **Own tables and id until epic 06.** Cheap to undo: the project item will reference the `img_` id.
4. **Layer names `layer.set_text` and `layer.set_shape`** because the video names `text.set` and `shape.set` take an item and have another schema.
5. **WebP through FFmpeg `libwebp`**, no new dependency, through the builder `ffargs.WebPStill` (D-093); `webp_unavailable` when the build has no encoder. PNG and JPEG from the standard library.
6. **Group opacity multiplies the children's.** The approximation is stated; isolated groups come with epic 29.
7. **Text height is measured by the caller's measurer** and stored, so a replay of the log gives the same document.
8. **Epic 29: one filter per kind, in a list.** The kind names a filter, so no id is needed and the order is the only other thing to change; two blurs in a row are one blur with a larger radius. Mask rectangles are thousandths of the box because the contract of SDD-004 section 3.9 already works in thousandths. A mask from another layer is a clip on the plan, so the renderer needs no second picture of the document. Groups stay without a look (isolated groups are a later epic).
9. **The lookup table is a key of `layer.adjust` (epic 49, D-093).** Resolved: an image names a `lut` asset by its content hash, which the service resolves for the document (`AssetInfo.Kind`) and `assets` lists with `kind` `lut`, and the plan carries the field the video plan already had. It is not an adjustment layer: a new kind of layer would change section 2 and the plan, and the renderer needs no new concept. The `.cube` parser, its limits and its arithmetic are those of SDD-004 section 3.6, unchanged; nothing of the 3D table is computed outside `engine/lut` and the renderer.
