# SDD-013: The renderer: plan input, pixel contract, host interface

- Status: Draft (2026-11-16). Written by P5-RND-001; the code follows in the later tasks of Phase 5 (stages C and D of ADR-0029).
- Depends on: ADR-0029 (one plan, one renderer, two hosts), ADR-0003 (one plan, two executors), ADR-0004 (time), ADR-0012 (rasterizer), ADR-0013 (browser stack), SDD-000 (names), SDD-004 (the plan, the compositor, the preview executor, tolerance), SDD-011 (the proxy), SDD-012 §11 (the preview component), SDD-007 (limits and process policy)
- Implements: ADR-0029 items 1 to 5; NFR-003, NFR-004, NFR-007, NFR-010
- Owner area: senior-b (`renderer/`, `engine/`), with `core/plan` owned by senior-a and the preview worker (`web/resources/js/editor/preview/`) by senior-e

## Contents

1. Overview
2. The frame plan as the renderer reads it
3. The pixel contract
4. The host interface
5. Frames from the hosts
6. The proxy chunk index
7. Errors and limits

## 1. Overview

ADR-0029 puts one renderer, written in Rust and compiled to WebAssembly, behind two hosts: the browser preview (a worker) and the Go export (`wazero`). This SDD is the contract between the renderer and everything around it. It does not change what a frame contains: `core/plan` decides that, and SDD-004 §2 to §3 stay the specification of the plan. It does not change the pixels either, until the stages of ADR-0029 say so: the rules of SDD-004 §6 are the reference of the CPU path, and §3 here restates them as the exact arithmetic the Rust code must reproduce.

What the renderer does: given one frame plan, the pixels each layer needs and the output size, it writes the frame as RGBA8. What it never does: read the document, interpolate a keyframe, choose a source frame, evaluate a transition, read a file, open a connection or read the clock (the threat row that ADR-0029 adds to SDD-007).

| Stage of ADR-0029 | What this SDD fixes for it |
| --- | --- |
| B | the chunk index the engine writes for WebCodecs (§6) |
| C | §2 (input), §3 (CPU path), §4 (module and `wazero` host), §5 (frames from FFmpeg), §7 |
| D | §3.8 (tolerance of the GPU path), §4 (browser worker), §5 (frames from WebCodecs) |
| E | the bitmap input of §2.5 is replaced by the plan's `BitmapSpec` alone |

## 2. The frame plan as the renderer reads it

### 2.1 Form and version

The renderer reads **JSON** (UTF-8, RFC 8259), the same JSON that `core/plan` already produces for `FramePlan` (SDD-004 §2) and that `GET /api/v1/projects/{id}/plan` carries inside its windows (SDD-004 §10). A binary form was weighed and not chosen for version 1: a plan of ten layers is about 2 KB, its parse is far below the budget of one frame at 60 fps, and one form that every host, test and golden file can print and diff is worth more than the microseconds. If a measurement ever shows the parse in a profile, the next version may add a binary form of the same fields; the version number below is how a host learns which one it has.

One call renders one frame. The input is this object (the **render input**):

```json
{
  "version": 1,
  "output": {"width": 540, "height": 960, "scale": 0.5},
  "plan": {
    "frame": 120, "width": 1080, "height": 1920, "background": "#000000",
    "layers": [
      {
        "item": "itm_m1n2p3q4r5", "kind": "media", "asset": "ast_3c5v8n2w6d", "asset_kind": "video",
        "source_frame": 62, "crop": {"x": 0, "y": 0, "w": 1080, "h": 1920},
        "color_matrix": null, "filter": null, "blur": 0,
        "cx": 540, "cy": 960, "w": 1080, "h": 1920, "rotation": 0, "scale": 1, "opacity": 1
      }
    ]
  }
}
```

- `version` is the integer `RenderInputVersion` (SDD-000 §12), 1 in this SDD. A renderer that does not know the number refuses the call with `unsupported_version` (§7) and never guesses. A new field, a changed rule or a changed default that alters a single pixel is a new version; a field that no pixel depends on (a name, an id) may be added without one. Host and module ship in one build, so a mismatch can only come from a stale cache, and the answer is to reload.
- `output.width` and `output.height` are the pixels of the frame to write, whole numbers from 1 to 4096 each (§7). `output.scale` is `outScale` of SDD-004 §6: every position and size of the plan is multiplied by it. The host computes `width = round(plan.width x scale)` and `height = round(plan.height x scale)` with `round` as in SDD-004 §6 (half up), except for the one caller that owns the rounding of the height (the size table of SDD-003 §3.11), which passes both; the renderer does not recompute them and does not check the relation, because the preview draws the paused frame at a width that is not a divisor of the project's.
- `plan` is the `FramePlan` of SDD-004 §2 with exactly the JSON names of `core/plan` (`json:` tags of `plan.FramePlan`, `plan.Layer`, `plan.Rect` and `plan.BitmapSpec`). It is not a second schema: the Go type is the schema, `core/schema` publishes it, and the Rust types are generated from or checked against that schema by a test.

### 2.2 The layer

Every number is a JSON number read as an IEEE 754 double (`f64`, correctly rounded decimal to binary conversion, as Go's `strconv` and Rust's `str::parse` both do). The plan already rounds every floating-point value to 3 decimal places (SDD-004 §2, stability), so the same text gives the same double in every host.

| Field | Type | Meaning |
| --- | --- | --- |
| `item` | string | the item id; it names the pixels of a media layer (§5) and appears in errors |
| `kind` | `"media"` or `"bitmap"` | where the pixels come from; any other value is `unsupported_layer` |
| `asset`, `asset_kind` | string | media layers: the asset and `"video"` or `"image"`; informational for the renderer |
| `source_frame` | integer | media layers: the frame of the normalized media; informational, the host has already fetched it |
| `crop` | `{x, y, w, h}` | media layers: the source rectangle in source pixels, already cut by the item's crop fractions; absent for a bitmap |
| `bitmap` | `BitmapSpec` | bitmap layers: what to draw; its `key` is `BitmapSpec.Key()` (§2.5) |
| `color_matrix` | 12 numbers or `null` | the 3x4 colour matrix of SDD-004 §3.5, row-major, offset in the fourth column |
| `filter` | 12 numbers or `null` | the filter matrix of SDD-004 §3.6, same form |
| `blur` | number | the standard deviation of the Gaussian of SDD-004 §3.6 in project pixels; 0 or absent for none |
| `cx`, `cy` | number | the centre of the drawn rectangle in project pixels |
| `w`, `h` | number | its size before rotation, in project pixels, scale already included |
| `rotation` | number | degrees clockwise |
| `scale` | number | the effective scale, informational (`w` and `h` already include it) |
| `opacity` | number | 0 to 1 |

Rules of reading:

1. **Order is the stacking order**: the first layer is the bottom. The renderer draws in array order and never sorts.
2. **Unknown fields are refused** (`invalid_plan`), not ignored: an ignored field is a pixel the renderer silently does not draw. The exception is the informational fields of the table (`asset`, `asset_kind`, `source_frame`, `scale`), which must be accepted and may be absent.
3. **Absent is zero or null**: `blur`, `color_matrix`, `filter`, `crop` and `bitmap` absent mean none. `opacity` has no default: a layer without it is `invalid_plan`. A number that is `NaN`, infinite or outside the double range cannot occur in JSON and is refused if a host builds the text by hand.
4. **Counts are bounded** (§7): at most 256 layers and 12 numbers in a matrix, else `limit_exceeded`.

### 2.3 What is not in the plan

- **Transitions.** `fade` is already in `opacity` and the slides are already in `cx` and `cy` (SDD-004 §3.2). The renderer receives no transition kind, no progress and no second layer to cross-fade; a transition is an ordinary layer whose numbers are those of this frame. This is what keeps the renderer from deciding anything an agent could see differently from a person (ADR-0029 item 1).
- **Keyframes and easing.** Evaluated by `core/plan` (SDD-004 §3.1); the plan holds the value at this frame.
- **Speed, trims and source time.** Evaluated by the plan (§3.3 of SDD-004); the host fetches the right source frame and hands over its pixels.
- **Audio.** The renderer draws pictures only.

### 2.4 From a window to a frame

The preview does not request one plan per frame; it requests a window (SDD-004 §10), where a constant is one number and a varying value is an array indexed by `frame - first`. The host expands a window to the render input of frame `t` with this rule, which is the whole of it: for every layer with `first <= t <= last`, take each of `cx`, `cy`, `w`, `h`, `rotation`, `scale` and `opacity` as the number or as the element `t - first` of the array, take `bitmaps.keys[bitmaps.index[t - first]]` as the key of a bitmap layer (and the spec of that key from the bitmap's own fetch, §2.5), copy `crop`, `color_matrix`, `filter` and `blur`, drop `source_seconds` (the host used it to choose the video frame, §5), and keep the layers in window order. A layer outside `[first, last]` is absent. The expansion will exist once in Go (a `Frame(t)` method on `plan.WindowPlan`) and once in TypeScript in the preview worker; both are tested against `plan.Frame(doc, t)` for every `t` of the golden documents, byte for byte after re-encoding, so the two cannot drift.

### 2.5 Bitmaps until stage E

Until the text rasterizer moves into the renderer (stage E), a bitmap layer's pixels come from `engine/raster` through the host: a `BitmapSpec` is rasterized to a `raster.Bitmap` (straight-alpha RGBA8, `Width`, `Height`, `NaturalW`, `NaturalH`, `CenterDX`, `CenterDY`; `engine/raster/types.go`), and the host hands it to the renderer under the key `BitmapSpec.Key()` together with the metrics. The renderer premultiplies it once per key (§3.2), places its centre at `cx - CenterDX x s`, `cy - CenterDY x s` with `s = w / NaturalW` (SDD-004 §6 step 2.1), and caches by key. In the browser the bitmap comes from `GET /api/v1/projects/{id}/bitmaps/{key}` (SDD-004 §10) and in the Go export from `raster.Cache`; the metrics travel with the bitmap in both. At stage E the spec in the plan is enough and the host supplies nothing; the key stays the cache key.

### 2.6 Acceptance of this section

- `TestRenderInputRoundTrip` (Go): the render input built from every golden plan of SDD-004 §11 decodes with `DisallowUnknownFields` and encodes to the same bytes.
- A schema test, written with the Rust types: the JSON Schema published for `plan.FramePlan` and the Rust types accept the same set of documents, and both refuse a layer with an unknown field, a missing `opacity` and a `kind` of `"video"`.
- `TestWindowFrameEqualsFrame` (Go) and its TypeScript twin: the expansion of §2.4 equals `plan.Frame` for each frame of each golden document.
