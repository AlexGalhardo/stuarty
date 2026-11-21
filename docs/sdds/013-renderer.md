# SDD-013: The renderer: plan input, pixel contract, host interface

- Amended (2026-11-18) for Stuarty: the same renderer draws image documents, as a one-frame plan built by `core/` (ADR-0038). The epic of the image editor adds what an image needs (more blend modes, adjustments, masks, image decoding and encoding) to this document before code.
- Status: Draft (2026-11-16). Written by P5-RND-001; the code follows in stages C and D of ADR-0029.
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
- `plan.frame` (0 to 2^32 - 1), `plan.width` and `plan.height` (not negative) are no longer informational (TASK-0279): the grain reads the frame and the whip the project size (SDD-004 §3.13 and §3.2); absent they are 0.
- `plan.transparent` is optional (SDD-004 §6, VV-172). Absent or false fills the frame with the opaque background. True fills it with that colour at alpha 0, so uncovered pixels stay transparent. It is a boolean; anything else is `invalid_plan`.
- `output.width` and `output.height` are the pixels of the frame to write, whole numbers from 1 to 4096 each (§7). `output.y0` and `output.y1` are optional: when present the renderer draws only the rows `[y0, y1)` of that frame and writes only them (the Go export gives each band its rows, §4.6); without them it draws every row. `output.scale` is `outScale` of SDD-004 §6: every position and size of the plan is multiplied by it. The host computes `width = round(plan.width x scale)` and `height = round(plan.height x scale)` with `round` as in SDD-004 §6 (half up), except for the one caller that owns the rounding of the height (the size table of SDD-003 §3.11), which passes both; the renderer does not recompute them and does not check the relation, because the preview draws the paused frame at a width that is not a divisor of the project's.
- `plan` is the `FramePlan` of SDD-004 §2 with exactly the JSON names of `core/plan` (`json:` tags of `plan.FramePlan`, `plan.Layer`, `plan.Rect` and `plan.BitmapSpec`). It is not a second schema: the Go type is the schema, `core/schema` publishes it, and the Rust types are generated from or checked against that schema by a test.

### 2.2 The layer

Every number is a JSON number read as an IEEE 754 double (`f64`, correctly rounded decimal to binary conversion, as Go's `strconv` and Rust's `str::parse` both do). The plan already rounds every floating-point value to 3 decimal places (SDD-004 §2, stability), so the same text gives the same double in every host.

| Field | Type | Meaning |
| --- | --- | --- |
| `item` | string | the item id; it names the pixels of a media layer (§5) and appears in errors |
| `kind` | `"media"` or `"bitmap"` | where the pixels come from; any other value is `unsupported_layer`. The plan also has `"group"` (SDD-004 §3.16); the host turns it into a media layer before the call (§3.12), so the module never receives one |
| `asset`, `asset_kind` | string | media layers: the asset and `"video"` or `"image"`; informational for the renderer |
| `effects` | array | the pixel effects of SDD-004 §3.13 in order (`kind` and every parameter of the kind; at most 6, `limit_exceeded` above; an unknown kind or a shake is `invalid_plan`) |
| `transition` | object or null | the whip, dip or wipe in progress (`kind`, `edge`, `p` 0 to 65536, `direction`, `color`), SDD-004 §3.2 |
| `source_frame` | integer | media layers: the frame of the normalized media; informational, the host has already fetched it |
| `crop` | `{x, y, w, h}` | media layers: the source rectangle in source pixels, already cut by the item's crop fractions; absent for a bitmap |
| `bitmap` | `BitmapSpec` | bitmap layers: what to draw; the renderer checks only its `kind` (`text`, `caption` or `shape`, else `unsupported_layer`) and leaves the rest of the object alone until stage E (§2.5) |
| `bitmap_key` | string | bitmap layers, added by the host to the plan's layer: `BitmapSpec.Key()` (§2.5), 1 to 64 ASCII bytes, the key under which the host stored the bitmap. The renderer does not hash the spec: it cannot reproduce Go's canonical JSON, and the host already has the key. A bitmap layer without it, or a media layer with it, is `invalid_plan` |
| `color_matrix` | 12 numbers or `null` | the 3x4 colour matrix of SDD-004 §3.5, row-major, offset in the fourth column |
| `filter` | 12 numbers or `null` | the filter matrix of SDD-004 §3.6, same form |
| `grade` | `{hsl, wheels, curves}` or `null` | the advanced colour of SDD-004 §3.14, the JSON of `plan.GradeSpec`: `hsl` is a list of at most 8 `{range, hue, saturation, lightness}` (a number that is absent is 0), `wheels` has `shadows`, `midtones` and `highlights` as `{hue, amount, luma}`, `curves` has `master`, `red`, `green` and `blue` as lists of at most 8 `{x, y}`; more is `limit_exceeded`, an unknown range or field `invalid_plan` |
| `lut` | `{asset, intensity}` or `null` | the lookup table of SDD-004 §3.6: `asset` (1 to 64 ASCII bytes) is the id the host gave to `vr_put_lut` and `intensity` is 0 to 1; a table that was not uploaded is `missing_texture` with the asset as the detail, unless the intensity rounds to 0 thousandths (then nothing is drawn and nothing is needed); an image layer carries it too (`adjust.lut` of SDD-021 section 3.1, asset the content hash, intensity the percent over 100 rounded to three decimals), on any layer that is not a group, and the table is the same `.cube` and the same arithmetic as for a clip |
| `chroma_key` | `{color, similarity, smoothness, spill}` or `null` | the key of SDD-004 §3.7, all four members required; absent or `null` means none |
| `blend` | string | the blend mode of SDD-004 §3.8: `multiply`, `screen`, `overlay` or `add`, and since epic 29 `soft_light`, `hard_light`, `darken`, `lighten`, `color_dodge`, `color_burn` or `difference` (§3.11.2); absent means normal (source-over of §3.7) |
| `tone` | 256 integers or `null` | the tone table of §3.11.1, each entry 0 to 255; absent or `null` means none |
| `filters` | array or `null` | the ordered pixel filters of §3.11.3 (`kind` and the members of the kind; at most 8, `limit_exceeded` above) |
| `clip` | `{source, invert}` or `null` | the mask from the alpha of another layer (§3.11.4): `source` is the `item` of that layer, `invert` a boolean that may be absent |
| `stencil` | boolean | the layer is read by a `clip` and drawn by nobody (§3.11.4); absent means false |
| `flip_x`, `flip_y` | boolean | mirror the layer across its own vertical or horizontal axis (SDD-004 §3.10); absent means false |
| `mask` | `{shape, x, y, width, height, feather, invert}` or `null` | the mask of SDD-004 §3.9: `shape` is `rect`, `ellipse` or, since epic 29, `rounded_rect` (with `radius`, in project pixels, an integer, absent means 0), `x`, `y`, `width`, `height` are fractions of the layer rectangle, `feather` is in project pixels (an integer), `invert` is a boolean and may be absent; absent or `null` means none |
| `shadow` | `{color, opacity, dx, dy, blur}` or `null` | the drop shadow of SDD-004 §3.15; every member is required when the object is present; absent or `null` means none |
| `outline` | `{color, width}` or `null` | the outline of SDD-004 §3.15; both members are required when the object is present; absent or `null` means none |
| `blur` | number | the standard deviation of the Gaussian of SDD-004 §3.6 in project pixels; 0 or absent for none |
| `cx`, `cy` | number | the centre of the drawn rectangle in project pixels |
| `w`, `h` | number | its size before rotation, in project pixels, scale already included |
| `rotation` | number | degrees clockwise |
| `scale` | number | the effective scale, informational (`w` and `h` already include it) |
| `opacity` | number | 0 to 1 |

Rules of reading:

1. **Order is the stacking order**: the first layer is the bottom. The renderer draws in array order and never sorts.
2. **Unknown fields are refused** (`invalid_plan`), not ignored: an ignored field is a pixel the renderer silently does not draw. The exception is the informational fields of the table (`asset`, `asset_kind`, `source_frame`, `scale`), which must be accepted and may be absent.
3. **Absent is zero or null**: `blur`, `color_matrix`, `filter`, `crop`, `bitmap`, `shadow` and `outline` absent mean none. `opacity` has no default: a layer without it is `invalid_plan`. A number that is `NaN`, infinite or outside the double range cannot occur in JSON and is refused if a host builds the text by hand.
4. **Counts are bounded** (§7): at most 64 layers and exactly 12 numbers in a matrix, else `limit_exceeded` and `invalid_plan` respectively.

### 2.3 What is not in the plan

- **Transitions.** `fade` is already in `opacity` and the slides are already in `cx` and `cy` (SDD-004 §3.2). The renderer receives no transition kind, no progress and no second layer to cross-fade; a transition is an ordinary layer whose numbers are those of this frame. This is what keeps the renderer from deciding anything an agent could see differently from a person (ADR-0029 item 1).
- **Keyframes and easing.** Evaluated by `core/plan` (SDD-004 §3.1); the plan holds the value at this frame. This includes the Bezier easing of epic 31: the fixed-point evaluation lives in the core only, the renderer, the compositor and the PixiJS preview receive the sampled `opacity`, `scale`, `cx`, `cy`, `rotation`, `mask` and `crop` of the frame and have no curve code. A new easing therefore adds no field to the plan and needs no change in the renderer; the parity cases of `preview-export-parity` prove it.
- **Speed, trims and source time.** Evaluated by the plan (§3.3 of SDD-004); the host fetches the right source frame and hands over its pixels.
- **Audio.** The renderer draws pictures only.

### 2.4 From a window to a frame

The preview does not request one plan per frame; it requests a window (SDD-004 §10), where a constant is one number and a varying value is an array indexed by `frame - first`. The host expands a window to the render input of frame `t` with this rule, which is the whole of it: for every layer with `first <= t <= last`, take each of `cx`, `cy`, `w`, `h`, `rotation`, `scale` and `opacity` as the number or as the element `t - first` of the array, take `bitmaps.keys[bitmaps.index[t - first]]` as the key of a bitmap layer (and the spec of that key from the bitmap's own fetch, §2.5), copy `crop`, `color_matrix`, `filter` and `blur`, drop `source_seconds` (the host used it to choose the video frame, §5), and keep the layers in window order. A layer outside `[first, last]` is absent. The expansion will exist once in Go (a `Frame(t)` method on `plan.WindowPlan`) and once in TypeScript in the preview worker; both are tested against `plan.Frame(doc, t)` for every `t` of the golden documents, byte for byte after re-encoding, so the two cannot drift.

### 2.5 Bitmaps until stage E

Until the text rasterizer moves into the renderer (stage E), a bitmap layer's pixels come from the Rust rasterizer through the host (since TASK-0220 that is the text exports of §4.2.1, reached from Go through `engine/rasterize`): a `BitmapSpec` is rasterized to a `bitmap.Bitmap` (straight-alpha RGBA8, `Width`, `Height`, `NaturalW`, `NaturalH`, `CenterDX`, `CenterDY`; `engine/bitmap`), and the host hands it to the renderer under the key `BitmapSpec.Key()` together with the metrics. The renderer premultiplies it once per key (§3.2), places its centre at `cx - CenterDX x s`, `cy - CenterDY x s` with `s = w / NaturalW` (SDD-004 §6 step 2.1), and caches by key. In the browser the bitmap comes from `GET /api/v1/projects/{id}/bitmaps/{key}` (SDD-004 §10) and in the Go export from `rasterize.Cache`; the metrics travel with the bitmap in both. At stage E the spec in the plan is enough and the host supplies nothing; the key stays the cache key.

### 2.6 Acceptance of this section

- `TestRenderInputRoundTrip` (Go): the render input built from every golden plan of SDD-004 §11 decodes with `DisallowUnknownFields` and encodes to the same bytes.
- A schema test, written with the Rust types: the JSON Schema published for `plan.FramePlan` and the Rust types accept the same set of documents, and both refuse a layer with an unknown field, a missing `opacity` and a `kind` of `"video"`.
- `TestWindowFrameEqualsFrame` (Go) and its TypeScript twin: the expansion of §2.4 equals `plan.Frame` for each frame of each golden document.

## 3. The pixel contract

This section is the CPU path of the renderer, written as arithmetic. It restates SDD-004 §3.5, §3.6 and §6 and `engine/compositor` as of v0.17.0. Where this text and the Go code differ, the Go code and its golden frames are right until the gate of stage C says the Rust code equals them, and this text is fixed in the same pull request (AGENTS.md rule 4). The CPU path is the truth of ADR-0029 item 3: golden frames are byte for byte against it. The GPU path of §3.8 is held to it by a tolerance, never the other way round.

### 3.1 Pixels and colour

- **Format.** A frame is `width x height x 4` bytes, row after row from the top, no padding, channels in the order R, G, B, A, 8 bits each. WebAssembly is little endian and every multi-byte number of this contract has an explicit integer width, so no byte order shows.
- **Alpha.** The output frame is opaque: it starts as the project background (`#RRGGBB`; anything else is `#000000`, and an alpha in the colour is ignored) with A = 255, and every blend keeps A = 255 there. Sources (media frames, bitmaps) are **premultiplied**: `C' = div255(C x A)` per colour channel. A decoded video frame without alpha is opaque (A = 255 for every pixel), which lets a copy skip blending.
- **Colour space.** All arithmetic is on **non-linear (sRGB-encoded) 8-bit values**, with no gamma linearisation, as browsers blend by default and as the Go compositor does (SDD-004 §6 rule 4). The conversion from the normalized media's BT.709 limited-range YUV to RGB is not the renderer's: the host does it (§5), with the matrix and range that SDD-004 §6 names. The renderer never sees YUV.

### 3.2 Integer helpers

All helpers are exact integer operations or exactly rounded IEEE 754 binary64 operations, so they give the same bits on every system and in WebAssembly.

| Name | Definition |
| --- | --- |
| `div255(x)` | `((x + 128) + ((x + 128) >> 8)) >> 8`, which is `x / 255` rounded to nearest, exact for `x` from 0 to 65535, on `u32` or `i64` |
| `u8(v)` | `min(v, 255)` as a byte (callers guarantee `v >= 0`) |
| `clamp8(v)` | `min(max(v, 0), 255)` on `i64` |
| `round_half_up(v)` | `floor(v + 0.5)` as an integer; for placements and sizes |
| `round_half_away(v)` | `v.round()` in Rust, `math.Round` in Go: halves away from zero; for the opacity and for fixed-point coefficients |
| `mul(a, b)` | the product of two `f64`, never fused with an addition (§3.9) |

### 3.3 Order of operations for one frame

1. Fill the output with the background.
2. For each layer, bottom first, in array order:
   1. `op = round_half_away(min(max(opacity, 0), 1) x 255)`. If `op` is 0 the layer is skipped.
   2. Get the source. A media layer has the host's pixels (`W x H`, premultiplied RGBA, §5) and the placement `cx, cy, w, h` of the layer times `scale`, with `rot = rotation`. A bitmap layer has the bitmap premultiplied once per key and the placement `cx' = (cx - mul(CenterDX, s)) x scale`, `cy' = (cy - mul(CenterDY, s)) x scale`, `w` and `h` times `scale`, where `s = w / NaturalW` (1 when `NaturalW` is 0).
   3. If `w <= 0` or `h <= 0` after scaling, or the source has no pixel or a short buffer, the layer is skipped without an error.
   4. A bitmap whose size is at least twice the rounded drawn size in both directions is first box-filtered down to it (the 2 x 2 halving of `compositor.Resize`, then the sampler), so a small frame is not point-sampled from a large bitmap.
   5. The chroma key (SDD-004 §3.7; `engine/compositor/chroma.go` and `renderer/stuarty-renderer/src/chroma.rs` are the same integer arithmetic), then the colour matrix (§3.4), then the grade and the lookup table (§3.4), then the filter matrix (§3.4), then the blur (§3.5), each only when the layer has it. The mask (§3.7) is placed on the rectangle of step 2, before the blur grows it (`compose::MaskState`, `renderer/stuarty-renderer/src/mask.rs`, the same integer arithmetic as `engine/compositor/mask.go`; a number outside the range of a valid plan is clamped, never a panic). The pixel effects, the whip blur and the dip of SDD-004 §3.13 and §3.2 (`renderer/stuarty-renderer/src/effects.rs`, the same integer arithmetic as `engine/compositor/effects.go`) come right after the flip, and the wipe is a coverage of the mask (`MaskState` with an optional wipe; a wipe alone has no shape). Last, `flip_x` and `flip_y` mirror the prepared pixels (`compose::flipped`, an exact permutation of pixels, SDD-004 §3.10), after the blur and with the mask already placed, so the mask is not mirrored.
   6. Draw (§3.6) with `op`.

### 3.4 Colour matrix and filter

The matrix `m` is 12 numbers: `m[4i + j]` is the coefficient of channel `j` for output `i` and `m[4i + 3]` is the offset. Each coefficient is converted once to `q[k] = round_half_away(mul(m[k], 65536))` (16.16 fixed point, `i64`). For each source pixel with alpha `a`:

- `a == 0`: the pixel is copied unchanged.
- Otherwise take the straight colour `(r, g, b)`: as it is for `a == 255`, else `min((c x 255 + a / 2) / a, 255)` with integer division.
- `nr = clamp8((q[0] x r + q[1] x g + q[2] x b + q[3] x 255 + 32768) >> 16)`, and `ng`, `nb` the same with `q[4..7]` and `q[8..11]`. The shift is arithmetic on `i64`.
- For `a != 255` premultiply again: `n = div255(n x a)`. Alpha is unchanged.

A coefficient is clamped to the range of 2^24 in absolute value after the rounding, so no sum can overflow; the plan never produces one that large, and the Go compositor is the same in that range. The result goes to scratch memory, never over the cached source. The filter matrix runs the same routine on the output of the colour matrix.

**Grade (TASK-0254).** The HSL bands, the wheels and the curves of SDD-004 §3.14 are drawn between the two matrices by `compose::prepare_layer` (`grade.rs`), in the integer arithmetic that section gives, on the straight colour with the same unpremultiply and premultiply as the matrix. Floating point is used only to turn a parameter of the plan into its integer (a product by a power of two and `round`), never per pixel. The GPU path draws the pixels this step prepares, as it does for the matrices, so it has no shader for the grade and needs no tolerance for it; `TestRustCPUPathEqualsGoCompositor` holds the four golden cases `grade-hsl`, `grade-wheels`, `grade-curves` and `grade-all` byte for byte.

**Lookup table (TASK-0254).** The trilinear lookup of SDD-004 §3.6 is `lut.rs` (`apply_lut`), run by `compose::prepare_layer` after the grade and before the filter, in the integer arithmetic of that section; the Rust side never parses a `.cube` file: the host reads it with `engine/lut` and uploads the entries as little-endian 16-bit integers (`vr_put_lut`, §4.2), like a font. The GPU path draws the pixels this step prepares, so it has no shader for it and needs no tolerance; the GPU child takes the table with the request of kind 5 (§4.9), and `TestRustCPUPathEqualsGoCompositor` holds the golden cases `lut-warm`, `lut-teal-half` and `lut-with-grade` byte for byte.

### 3.5 Blur

Exactly SDD-004 §3.6, "Implementation". `sigma = mul(blur, scale)`; `r = round((sqrt(4 sigma^2 + 1) - 1) / 2)` (the square root is exactly rounded in every host), capped at 128, and `r = 0` means no blur. The layer is padded by `3r` transparent pixels on every side, blurred by three box passes along x and then three along y in integer arithmetic that rounds half up at every pass (16-bit lanes for the window sums, the reason for the cap), and drawn with a placement that grows by `3r` around the same centre. The reference for every byte is `engine/compositor/blur.go` and the golden cases `blur-2`, `blur-6`, `blur-16` and `blur-still`; the Rust port is correct when those four are byte-equal.

### 3.6 Placement and drawing

The placement is a `w x h` rectangle centred on `(cx, cy)` turned by `rot` degrees clockwise, in output pixels.

- **Copy path.** When `rot == 0` and the source's width and height equal `round_half_up(w)` and `round_half_up(h)`, the source is copied one to one with its top-left corner at `(round_half_up(cx - w/2), round_half_up(cy - h/2))`, clipped to the frame, through the blend of §3.7.
- **Sampling path.** Otherwise each destination pixel whose centre `(x + 0.5, y + 0.5)` is inside the rectangle takes the four nearest source pixels with bilinear weights. Edges are not smoothed. The steps are:
  1. `(sin, cos) = sincos(mul(rot, pi / 180))` (§3.9).
  2. `hw = w / 2`, `hh = h / 2`. The box to scan is `[floor(cx - ex), ceil(cx + ex)) x [floor(cy - ey), ceil(cy + ey))` clipped to the frame, with `ex = mul(hw, |cos|) + mul(hh, |sin|)` and `ey = mul(hw, |sin|) + mul(hh, |cos|)`. An empty box draws nothing.
  3. With `kx = srcW / w`, `ky = srcH / h`, `dx0 = 0.5 - cx`, `dy0 = 0.5 - cy` and `q(v) = round_half_away(mul(v, 2^32))` as `i64` (32.32 fixed point): `ax = q(mul(cos, kx))`, `bx = q(mul(sin, kx))`, `ay = q(-mul(sin, ky))`, `by = q(mul(cos, ky))`, `ox = q(mul(mul(dx0, cos) + mul(dy0, sin) + hw, kx))`, `oy = q(mul(-mul(dx0, sin) + mul(dy0, cos) + hh, ky))`. Every product is rounded before it is added.
  4. For pixel `(x, y)`: `sx = bx x y + ox + ax x x` and `sy = by x y + oy + ay x x`, all `i64`. If `sx < 0`, `sy < 0`, `sx >= srcW << 32` or `sy >= srcH << 32` the pixel is not drawn. Else `fx = sx - 2^31`, `fy = sy - 2^31`, `x0 = fx >> 32`, `y0 = fy >> 32` (arithmetic shifts), `wx = (fx >> 24) & 255`, `wy = (fy >> 24) & 255`, `x1 = min(x0 + 1, srcW - 1)`, `y1 = min(y0 + 1, srcH - 1)`, then `x0 = max(x0, 0)` and `y0 = max(y0, 0)`.
  5. For each of the four channels: `top = p00 x (256 - wx) + p10 x wx`, `bot = p01 x (256 - wx) + p11 x wx`, `v = (top x (256 - wy) + bot x wy + 32768) >> 16`, on `u32`.
  6. Opacity and blend as in §3.7.

### 3.7 Blending

Source-over on premultiplied values. For a source pixel `(r, g, b, a)` and the layer opacity `op` from 0 to 255:

- `op == 255` and `a == 255`: the destination pixel is replaced.
- Otherwise, when `op != 255`: `a = div255(a x op)` and `c = div255(c x op)` for `r`, `g`, `b`. If `a == 0` nothing is drawn.
- With `ia = 255 - a`, each destination channel, alpha included, becomes `u8(c + div255(d x ia))`.

A layer with `blend` replaces the last bullet by the per-channel mix of SDD-004 §3.8 (same `op` handling, same `div255`, `a == 0` draws nothing, the destination alpha stays 255), and is never drawn by the replace shortcut of the first bullet.

A layer with `mask` multiplies each source pixel by its coverage `cov` of SDD-004 §3.9 (all four channels, `div255` each) before the `op` step, and is never drawn by the replace shortcut either; `cov == 0` draws nothing. The position of the pixel for the coverage comes from the placement of §3.6 (copy path: the rounded corner of the placement before any blur; sampling path: the transform of step 3 with `kx = ky = 1`).

### 3.8 The GPU path

The GPU path (stage D, `wgpu`: WebGPU in the browser, Vulkan, Metal or D3D12 natively) is not bit-exact and does not try to be: float arithmetic differs across vendors and drivers. It follows the same order of operations (§3.3) and is held to the CPU path by:

- **SSIM** of at least 0.99 on the luma of the whole frame, luma being BT.709 `0.2126 R + 0.7152 G + 0.0722 B` of the final 8-bit values, computed by the SSIM routine of the SDD-004 §11 tests;
- **geometry** within 0.5 pixel: for every layer of the golden cases, each of the four corners of the drawn rectangle, as the GPU draws it, differs from the CPU path's corner by at most 0.5 output pixel. The quads are transformed on the CPU in `f64` and handed to the GPU as final positions, so this checks the pipeline and is not a new freedom.

Both are checked on every golden case, in three browsers, and on the three operating systems for the native host. A case that fails is a bug of the GPU path; the CPU path is never loosened to match it.

**Implementation (P5-RND-005, native).** The crate `renderer/stuarty-renderer-gpu` (`wgpu` 30.0.2, Vulkan, Metal and Direct3D 12, no WebAssembly yet) depends on `stuarty-renderer`, so the two paths share the plan decoder, the stored textures and bitmaps (`Renderer::layers`), the preparation of each layer (`compose::prepare_layer`: shrink of a large bitmap, chroma key, colour matrix, filter and blur, still on the CPU), the opacity (`layer_opacity`) and the choice of the one-to-one copy (`compose::copy_origin`). The GPU decides the sampling and the blend: one quad per layer, positioned by `quad_for` (the rounded integer rectangle for a copy, else the rectangle turned about its centre, in `f64`, handed over as `f32` positions), a WGSL fragment shader that interpolates the four nearest texels with the clamped edges of section 3.6, and the premultiplied source-over of section 3.7 as the fixed blend state of the pipeline. A layer with a blend mode (SDD-004 §3.8; overlay is not a fixed-function blend) is drawn by a second pipeline whose fragment shader reads the frame from a backdrop texture and writes the mixed pixel with the blend state off: before such a layer the frame is copied to the backdrop (`copy_texture_to_texture`), so the frame is one clearing pass and one pass per layer, and the shader evaluates the formulas of §3.8 in `f32` (the sources are the same premultiplied, opacity-weighted values). A layer with a mask (SDD-004 §3.9) gets a uniform from `MaskState::geometry` (the shape in pixels of the layer's own rectangle and the map from a pixel to its position in it, one-to-one for a copy and the CPU sampler's transform with `kx = ky = 1` otherwise); the fragment shader computes the coverage in `f32` (distance to the edge, the same feather ramp, invert) and multiplies the sampled pixel by it before the opacity and the blend. The rows of a band are a scissor rectangle. The native gate is `cargo test -p stuarty-renderer-gpu` (`tests/parity.rs`): each of the 44 vectors of `renderer/testdata/compose-vectors.bin` is drawn by both paths; the luma SSIM (the routine of `engine/export/util_test.go`), the best whole-pixel displacement of the frame (search of plus or minus 2, zero required) and the distance of every quad corner from the corner found by inverting the CPU sampler are asserted and printed with the adapter; `STUARTY_TEST_GPU_ADAPTER=software` selects the software adapter, and a machine with no adapter prints a SKIP. The flip (SDD-004 §3.10) needs no shader work: `prepare_layer` hands the GPU the mirrored pixels, so the texture is already flipped. On Windows, with the blend, mask and flip vectors: NVIDIA RTX 4060 (Vulkan) worst SSIM 0.99860, Microsoft Basic Render Driver (Direct3D 12, software) worst SSIM 0.99871 (the three blend vectors 0.99944 and above, the four mask vectors 0.99991 and above, the three flip vectors 0.99991 and above), all corners within 0.0001 pixel and no displacement; the browser part is TASK-0215. The pixel effects, the whip blur and the dip (SDD-004 §3.13 and §3.2) need no shader work either: `prepare_layer` draws them on the CPU, so the texture already has them. The wipe is a coverage: `MaskGeometry` carries its direction and its edge in pixels (`wipe`, and `shapeless` for a layer with a wipe and no mask), the uniform holds them in the spare components, and the fragment shader multiplies the coverage by a hard 0 or 1 (`u < edge`, `u >= edge`, `v < edge`, `v >= edge`). With the 44 vectors of version 6 (TASK-0279): NVIDIA GeForce RTX 4060 (Vulkan) worst SSIM 0.99693 and Microsoft Basic Render Driver (Direct3D 12, software) worst SSIM 0.99694, both on `effect-rgb-split` (a rotated, scaled layer whose channels are shifted); the other new vectors are 0.99985 and above (`effect-motion-blur` 0.99989, `effect-glow` 0.99996, `effect-vignette-grain` 0.99998, `effects-stacked-masked-flipped` 0.99986, `transition-whip` 0.99991, `transition-dip` 0.99996, `transition-wipe` 0.99985), no displacement and all corners within 0.0001 pixel. The core module of the CPU path does not depend on this crate (DS-1), and `internal/tools/notices` lists the crates of both the core module (wasm32) and this crate (the six platforms of the archives), SDD-009 section 12.2.

### 3.9 Determinism rules for the Rust code

These rules make §3.1 to §3.7 give the same bytes on Linux, macOS, Windows and in WebAssembly, for any number of bands:

1. **No `f32`** in the CPU path. `f64` only for the per-layer geometry; per-pixel work is integer.
2. **No fused multiply-add.** Every product that is later added goes through `mul`, which stores the product in a binding the compiler may not fuse across, and `f64::mul_add` is banned by a clippy lint. The build never enables `fma` as a target feature. WebAssembly has no fused operation; arm64 has one and fuses `a * b + c` unless told not to (the Go code has the same rule for the same reason).
3. **No platform math library.** `f64::sin`, `cos`, `powf`, `exp`, `ln`, `tan` and `atan2` are not used: they come from the platform's `libm` and differ in the last bit. `sincos` is the series of `engine/compositor/compositor.go`: reduce with `x -= mul(2 pi, floor(x / (2 pi) + 0.5))`, then 24 terms of the Taylor series, computed with `mul` and divisions only. `sqrt`, `floor`, `ceil`, `abs`, `round`, `+`, `-`, `*` and `/` are exactly rounded by IEEE 754 and are allowed.
4. **Integer widths are explicit.** `i64` for fixed-point sums, `u32` for pixel arithmetic; shifts of signed values are arithmetic; no `as` conversion from a float that can overflow (clamp first); no reliance on wrapping.
5. **No threads in the module.** The module is single threaded. The Go host may run several instances on disjoint bands of the frame (rows `[y0, y1)`); the result of a row never depends on another row's work, so one band or eight give the same bytes (SDD-004 §12 rule 3).
6. **No SIMD with different rounding.** Auto-vectorised integer code is fine. A SIMD routine (`simd128`) is allowed only when a test proves it equal to the scalar routine over its whole domain (for `div255`, all 65536 inputs) or, when the domain is too large, over a sample set that covers every branch.
7. **No clock, no random numbers, no hash-map iteration order** in anything that influences a pixel.

### 3.10 Acceptance of this section

- Stage C gate: every golden case of SDD-004 §11 (the `requiredCases`, `sticker-png`, `sticker-anim`, `filter-*` and `blur-*`) composed by the Rust CPU path through `wazero` equals the stored frame byte for byte, at 1 and at 8 bands.
- `div255` equals `x / 255` rounded to nearest for every `x` from 0 to 65535, in Rust and in Go; `sincos` equals the Go `sincos` bit for bit over 10,000 angles in `[-1000, 1000]` and at every multiple of 15 degrees.
- The same Rust test suite runs natively on the three operating systems and in WebAssembly under `wazero`, and prints the same SHA-256 of the golden frames.
- Harness (P5-RND-003): `TestRustCPUPathEqualsGoCompositor` (`engine/export/parity_rust_test.go`) records, for every frame of the 37 golden cases and of 3 reference scenes, the plan and the pixels the Go compositor was given, builds the render input of §2.1, renders it with the native process `vr-parity` of `renderer/` (`cargo build --release --bin vr-parity`, or `STUARTY_TEST_VR_PARITY`), once whole and once in eight bands (`output.y0`, `output.y1`), and requires the same bytes. The same frames are then drawn by the embedded WebAssembly module under `wazero` through `engine/renderhost` (§4.6), whole with one instance and in eight bands with one instance per band, and must be the same bytes: 170 frames, 0 bytes differ on every one of the four runs.
- Fuzz (P5-RND-003): `renderer/stuarty-renderer/tests/fuzz.rs` runs 100,000 random byte strings and 200,000 mutated plans through the decoder, 6,000 compositions with the extremes of a double (`NaN`, infinities, `f64::MAX`, empty and huge rectangles, bands outside the frame), 30,000 sampler placements, 50,000 mutated renders (each repeated and compared) and 20,000 store calls with every size and length; a panic or an arithmetic overflow fails the test. The generator is a fixed-seed xorshift written in the test (`proptest` and `cargo-fuzz` are not approved crates); `VR_FUZZ_ITERATIONS` scales a long run.
- Vectors, format version 6 (TASK-0279): each case also stores the plan's frame number and the project size, and each layer its effects (kind, parameters) and its whip, dip or wipe (kind, edge, `p`, direction, colour); eight vectors were added (the four effects, the five effects stacked with a blur, a flip and a mask, and the whip, the dip and the wipe), 44 in all.
- Vectors (P5-RND-003): `renderer/testdata/compose-vectors.bin` holds 36 small compositions (copy, clip, opacity, scale, rotation, colour matrix, filter, blur at three radii, bitmaps with metrics, the shrink of a large bitmap, a stack, four chroma keys, three blend compositions, four mask compositions and three flip compositions) with the frame `engine/compositor` draws for each; `cargo test` draws them again at 1 and 8 bands and requires the same bytes, and `TestRendererVectorsAreCurrent` keeps the file equal to the Go output. The FNV-1a hash of `sincos` over its 10,001 + 97 angles is stored in the same file.
- Rasterizer vectors (stage E, TASK-0219, reference since TASK-0220): `renderer/testdata/raster-vectors.bin` holds the specs and the bitmaps the Rust rasterizer draws (format `VRAS`, version 2: the pixels are stored as runs of zero bytes and of other bytes, and a text case also stores the boxes of its words). It has 56 cases: the shapes (three goldens and seven edge cases), the 16 text and caption goldens, and 15 corner cases of the layout, the highlight and the effects (empty lines, a trailing space, a word wider than the box, the three alignments, a fractional size at scale 1.5, a shadow with an offset and a blur, a box, a thick stroke, upper-casing, a ligature and a kern, a forced break, a highlight colour, no highlight, a narrow box), each drawn in Inter, a variable font, and in Anton, a static one. `cargo test -p stuarty-renderer-text --test raster_vectors` draws each case again and requires the same bytes, metrics and word boxes; `VUKEM_WRITE_RASTER_VECTORS=1` rewrites the bitmaps (a reviewed change of output, and a new `BitmapVersion`). `TestRasterVectorsAreCurrent` (`engine/rasterize`) encodes the same specs, draws them through the embedded module under `wazero` and requires the file's bytes, so the native crate and the module cannot drift.
- Rasterizer crates (stage E, TASK-0219; the author chose them on 2026-11-16 because `rustybuzz` and `ttf-parser` are flagged unmaintained by `cargo deny`, RUSTSEC-2026-0206 and RUSTSEC-2026-0192): `harfrust` 0.16.0 (shaping, HarfBuzz 15.1, at the scale `ceil(size) * 64` that go-text uses, so the advances come out in the same 26.6 fixed point), `skrifa` 0.50.0 (font parsing, outlines in font units and the metrics of the lines), `unicode-bidi` 0.3.18, `unicode-linebreak` 0.1.5 and `unicode-script` 0.5.9, all at exact versions. `yoke`, `yoke-derive`, `zerofrom` and `zerofrom-derive` come with them and are licensed `Unicode-3.0` only, so they are per-crate exceptions next to `unicode-ident` in `renderer/deny.toml` and in `internal/tools/notices/spdx.go`. The crate `stuarty-renderer-text` is linked into the module of §4.1 (the text exports of §4.2.1), so these crates are part of the shipped notices.
- Rasterizer reference (stage E, TASK-0220; decision of the author, 2026-11-16): the Rust output is the reference. While the port was checked against `engine/raster`, the shapes, the static fonts and 14 of the 15 corner cases in Anton were byte for byte the Go bitmaps (26 of the 56 vectors) and 30 differed: go-text 0.3.5 did not apply the kerning of most pairs of Inter, Montserrat and Playfair Display (`Wo`, `VA`, `To`, `LT`) that `harfrust` applies, so a line was a few pixels narrower (`text-ligature-and-kerning` in Inter was 598 against 612 pixels); go-text scaled a GPOS value with a truncating integer division where HarfBuzz rounds (1/64 pixel); and it computed the points of a variable glyph in `float32` where `skrifa` uses 16.16 fixed point (one font unit). The author accepted the Rust output rather than keep go-text's shaping: the vectors and the goldens were regenerated from Rust, `plan.BitmapVersion` is 2 (every cached bitmap is invalidated) and `engine/raster` was removed. The kerning is now applied as the fonts say.
- Stage D gate: §3.8 on every golden case.

### 3.11 Image looks (epic 29)

Added by epic 29 (RF-333, RF-334, RF-335, ADR-0038) for the one-frame plans of image documents. Every field below is optional and absent means the old behaviour, so a plan that has none of them draws the bytes it drew before and `RenderInputVersion` stays 1 (a renderer older than this section refuses the plan with `invalid_plan`, as §2.2 rule 2 says, and host and module ship in one build). The Go compositor of the video export does not draw them: only the plans of image documents, drawn by the Rust renderer, carry them. **Video.** No video operation sets them yet: the default export of a video is the Go compositor, which would ignore them without an error, so a clip operation for them waits for the Rust renderer to be the default of the video export (the gate of ADR-0029 stage C), when each is one registration and one golden case on the plan fields below. The golden frames of this section are the golden cases of those operations: a video layer of the plan is the same layer. `core/image` builds them (SDD-021 section 5); the renderer decides nothing.

**Order for one layer.** §3.3 step 2.5 becomes: chroma key, the tone table (3.11.1), colour matrix, grade, lookup table, filter matrix, blur, **the filters of 3.11.3 in list order**, flip, pixel effects, shadow and outline, then the mask, the clip (3.11.4), opacity and the draw with its blend mode (3.11.2). A plan layer of an image uses `color_matrix`, `tone`, `lut` (epic 49: the table is drawn after the tone table and the colour matrix and before the filter matrix and the filters, and a table is uploaded with `vr_put_lut` before the layer is drawn), `filters`, `mask`, `clip` and `blend`; the other fields stay for video.

#### 3.11.1 The tone table

`tone` holds `T[0..255]`, built in Go (`core/image`) from the exposure, the levels and the curve of the document and passed in the plan, so the renderer has no formula to get wrong. For a pixel with alpha `a`: `a == 0` is copied; otherwise the straight colour is taken as in §3.4, each channel `c` becomes `T[c]`, and the pixel is premultiplied again with `div255` when `a != 255`. Alpha is unchanged. The table is built as three tables composed, each entry rounded half up to an integer in 0 to 255, and held to that range:

1. Exposure `e` (hundredths of a stop): `E[v] = clamp(round(v x 2^(e / 100)))`.
2. Levels `black`, `white`, `gamma` (hundredths): `L[v] = round(255 x (clamp((v - black) / (white - black), 0, 1))^(100 / gamma))`.
3. Curve, the points sorted by `x` with `(0, 0)` and `(255, 255)` added when `x = 0` or `x = 255` is missing: for `v` between two points `(x0, y0)` and `(x1, y1)`, `C[v] = y0 + round_half_away((v - x0) x (y1 - y0) / x1 - x0)` with the division of integers rounded to nearest, halves away from zero; `C[v] = y` at a point.

`T[v] = C[L[E[v]]]`, the steps absent when neutral. The Go function is pure and its output is the golden of the case; a curve through `(x, x)` points is the identity, and a monotonic input gives a table that is monotonic when the points rise (a property test).

#### 3.11.2 Blend modes

The destination pixel is `(dr, dg, db, da)`, the source is premultiplied with the opacity and the mask applied, `(r, g, b, a)`; `a == 0` draws nothing. The four modes of SDD-004 section 3.8 keep their formulas when `da == 255`, which is every plan with an opaque background. The seven new modes, and every mode but `normal` when `da < 255` (a transparent canvas), use the general form, on the straight values `s = min((c x 255 + a / 2) / a, 255)` of the source and `b = min((dc x 255 + da / 2) / da, 255)` of the destination (when `da == 0` the pixel is the source, as for `normal`):

```text
B     = blend function of s and b, 0 to 255 (table below)
t     = div255(a x da)
out_c = div255(c x (255 - da)) + div255(dc x (255 - a)) + div255(t x B)
out_a = a + da - t
```

`out_c` and `out_a` are clamped to 255. With `da == 255` this is `div255(dc x (255 - a)) + div255(a x B)`. The general form also draws `multiply`, `screen`, `overlay` and `add` on a transparent canvas, with `B` of the table; on an opaque one those four keep SDD-004 section 3.8, and `multiply`, `screen` and `overlay` agree with the general form within two levels (a property test); `add` differs by design, SDD-004 adding the premultiplied source without the clamp of `B`. The Go compositor (`engine/compositor/blend.go`) draws the same general form for these four modes when `da < 255`, `add` included (VV-257): the goldens `blend-multiply-transparent`, `blend-screen-transparent`, `blend-overlay-transparent` and `blend-add-transparent` hold it, and the parity gate of the renderer-parity skill compares them byte for byte. The seven new modes are drawn only by the Rust renderer. `mul(x, y) = div255(x x y)` and `scr(x, y) = x + y - mul(x, y)`:

| Mode | `B(s, b)` |
| --- | --- |
| `multiply` | `mul(s, b)` |
| `screen` | `scr(s, b)` |
| `overlay` | `hard_light` with `s` and `b` exchanged: `2 mul(s, b)` when `b <= 127`, else `scr(s, 2 b - 255)` |
| `add` | `min(s + b, 255)` |
| `darken` | `min(s, b)` |
| `lighten` | `max(s, b)` |
| `difference` | `abs(s - b)` |
| `hard_light` | `mul(b, 2 s)` when `s <= 127`, else `scr(b, 2 s - 255)` (the `mul` argument is held to 255) |
| `soft_light` | `s <= 127`: `b - div255((255 - 2 s) x mul(b, 255 - b))`; else `b + round((2 s - 255) x (D(b) - b) / 255)`, rounding half away from zero, with `D(b) = ((16 b' - 12) b' + 4) b'` for `b <= 64` taken in 8 bits as `(((16 b - 3060) x b + 260100) x b + 32512) / 65025` (integer division) and `D(b) = round(sqrt(255 b))` above 64 (the integer root, plus 1 when `255 b - r^2 > r`); the result is clamped to 0 to 255 |
| `color_dodge` | `0` when `b == 0`, `255` when `s == 255`, else `min(255, (b x 255 + (255 - s) / 2) / (255 - s))` |
| `color_burn` | `255` when `b == 255`, `0` when `s == 0`, else `255 - min(255, ((255 - b) x 255 + s / 2) / s)` |

These are the separable blend functions of the W3C compositing specification in 8-bit integers. The GPU path evaluates the same formulas in `f32` (the modes 5 to 11 of its shader) and is held to §3.8.

#### 3.11.3 Filters

`filters` entries, drawn in order on the layer's own pixels (each may pad the layer, like the blur, and the placement grows around the same centre). Lengths are project pixels, times `outScale`, as for the blur. Every colour is `#RRGGBB` or `#RRGGBBAA` (§3.15).

| `kind` | Members | Drawn as |
| --- | --- | --- |
| `matrix` | `matrix` (12 numbers) | the filter matrix of §3.4 |
| `blur` | `sigma` | the blur of §3.5 |
| `sharpen` | `amount` (0 to 3), `radius` (0 to 100, the standard deviation) | the unsharp mask below |
| `vignette`, `grain` | as the effects of SDD-004 §3.13 (`amount`, `softness` / `amount`, `size`, `seed`) | the same function, on the position of the layer's own rectangle |
| `shadow` | `color`, `opacity`, `dx`, `dy`, `blur` | the drop shadow of §3.15 alone |
| `stroke` | `color`, `width` | the outline of §3.15 alone |
| `outer_glow` | `color`, `radius` (the standard deviation), `amount` (0 to 4) | the glow below |

**Unsharp mask.** `rx`, `ry` are the box radii of the blur of §3.5 for `sigma = mul(radius, outScale)` (both 0 draws nothing). The premultiplied pixels are blurred by the three box passes of §3.5 with the edge pixels repeated beyond the layer instead of transparent ones (so an opaque picture has no bright fringe), and the layer keeps its size. With `ap = round_half_away(mul(amount, 1000))`, each colour channel `c` with blurred value `u` becomes `clamp(c + q, 0, a)` where `q = ((c - u) x ap + 500) / 1000` for a positive product and `-((u - c) x ap + 500) / 1000` for a negative one (integer division, half away from zero); alpha is unchanged.

**Outer glow.** `sigma = mul(radius, outScale)`; the silhouette (the alpha of the layer, colour 0) is blurred as in §3.5 (the layer grows by 3 radii on every side; a zero radius draws nothing), the blurred alpha `g` becomes `ga = div255(min(255, (g x ag + 500) / 1000) x ca)` with `ag = round_half_away(mul(amount, 1000))` and `ca` the alpha of the colour, the glow pixel is the colour premultiplied by `ga`, and the layer is drawn over it with the premultiplied source-over of §3.7: `out = c + div255(glow x (255 - a))` for every channel including alpha.

**Limits.** A shadow offset is held to 2048 pixels of the layer's own bitmap, so a plan that is not valid cannot ask for an unbounded padding (a valid one is far below it); the filters are fuzzed like the rest of the plan. The dilation of the outline (SDD-004 section 3.15) takes, for each row of the disc, the maximum of a window of the source row with a monotonic queue, so its cost is the pixels times the height of the disc and its bytes are those of the definition (a test compares the two); a stroke of 100 pixels on a large layer would otherwise cost the area of the disc for every pixel.

**Mask shape `rounded_rect`** (§3.9). The shape of §3.9 step 2 with the corner radius `rq = min(round_half_away(mul(mul(radius, outScale), 256)), a, b)` (24.8; `a`, `b` the half-extents), and in step 3, with `qx = abs(lx - (L + a)) - (a - rq)` and `qy = abs(ly - (T + b)) - (b - rq)`: `d = rq - max(qx, qy)` when both are at most 0, else `d = rq - isqrt(max(qx, 0)^2 + max(qy, 0)^2)`. Feather and invert are unchanged. `radius` 0 is the `rect` of §3.9.

#### 3.11.4 The clip

A layer with `clip` is drawn only where the layer named by `source` has pixels. The source is found by `item` among the layers of the plan (the layer list the renderer drew, a `stencil` one included); a source that is not there, or that has a `clip` itself, is `invalid_plan`. The coverage plane `S` is the alpha of the source drawn alone on a transparent frame of the same size: its own pixels prepared as in §3.3 (its own mask included, its opacity not), placed and sampled as in §3.6, with source-over. For a destination pixel `cov = S` (`255 - S` with `invert`), and it multiplies the pixel of the clipped layer, all four channels with one `div255` each, in the same place as the mask coverage and after it (`div255(mask x clip)` is applied as two products). A layer with a clip is never drawn by the opaque copy shortcut. A `stencil` layer is prepared like any other but is not drawn on the frame; its `opacity` is 1 (a layer that rounds to no opacity is skipped, so the plan gives a stencil the opacity 1). The GPU path receives the plane as a texture of the frame size and multiplies the coverage of the mask by it.

**Golden frames.** `renderer/testdata/image-ops/` holds, per operation of this section, a render input (`<case>.json`), the textures it draws (made by the pattern functions that `renderer/stuarty-renderer/tests/image_ops.rs` and `engine/renderhost/image_ops_test.go` both implement) and the frame (`<case>.rgba`, raw RGBA8). `cargo test` and the Go test through `wazero` draw each case and require the same bytes; `STUARTY_WRITE_IMAGE_OPS=1 cargo test --test image_ops` rewrites the frames (a reviewed change of output). The GPU path draws the same inputs and is compared by the rule of §3.8.

### 3.12 Groups (epic 32)

A nested composition that has an opacity, a blend, a mask, an effect or another property of its own is one layer of kind `group` in the plan (SDD-004 §3.16): `{"kind": "group", "item", "group": {"width", "height", "layers": [...]}, ...}` with the placement and look of a media layer of a picture of `width` by `height`. **The module never draws a group** (a plan that holds one is `unsupported_layer`, as for any kind it does not know): the renderer decides nothing and has no sub-composition. The host draws the group's surface with the module itself, then gives the module an ordinary media layer. This is the "intermediate surface" of ADR-0042, and it is the same two steps in every host, so the pixels agree.

For a group layer `l` at output scale `s` (the `output.scale` of §2.1; `s` is 1 for a view at the project size):

1. `k = s × l.w / group.width`, the factor from a nested pixel to an output pixel, lowered so that neither side of the surface passes the 4096 of §7.2: `k = min(k, 4096 / group.width, 4096 / group.height)`.
2. The surface is `sw` by `sh` pixels with `sw = max(round(group.width × k), 1)` and `sh = max(round(group.height × k), 1)` (`round` is round half up, SDD-004 §6).
3. The surface is one frame of the nested plan `{width: group.width, height: group.height, transparent: true, layers: group.layers}` drawn at output scale `k` into `sw` by `sh` pixels, by the rules of §3.3 to §3.11 (so a group in it is drawn by this section first), over a clear surface: alpha 0 where no layer draws, the pixels premultiplied. The background of the nested timeline is not used.
4. The layer is then drawn as a media layer whose texture is that surface, whose `crop` is the whole surface (`0, 0, group.width, group.height`) and whose `cx`, `cy`, `w`, `h`, `rotation`, `scale`, `opacity`, `blend`, `mask`, `filter`, `blur`, `flip_x`, `flip_y`, `shadow`, `outline`, `effects` and `transition` are the group layer's: the placement and sampling of §3.6 (the copy shortcut when `sw`, `sh` equal the rounded drawn size and the layer is not turned), then every step of §3.3 on the layer's own pixels. The texture is not opaque unless every alpha byte is 255.

A layer of the group whose opacity rounds to 0 is skipped before its pixels are fetched, as at the top level. At most 4 surfaces exist at once (the depth limit of SDD-001 §3.5.1), each at most 4096 by 4096 pixels.

**Where it is done.** The Go compositor (`Compositor.Compose`, which draws groups recursively) and the Go host of the module (`engine/export/rust_renderer.go`, for the CPU path and, through the same textures, the GPU path of §4.9) follow the four steps, and so do the export, `ViewFrame` and the paused frame of the editor, which the server draws with them. Go compositor and module give the same bytes (§3.10); the GPU path follows §3.8 and ADR-0003 item 6. The browser executors draw a group live (epic 52, author decision D-094): the renderer executor draws the surface with the module in its worker, in the same two steps (§4.7.1), and the PixiJS executor on a render texture, approximately; the paused frame still comes from the server (SDD-004 §3.16). **Golden frames**: `engine/export/testdata/golden-nested/` has a nested timeline drawn as a group with an opacity (so its layers do not show through each other), with a mask and a blend, scaled and turned with a shadow, with a `source_in`, and with an effect, drawn by both CPU paths (the Go compositor in one band and in the default bands, and the module) and required byte-equal; `TestNestedOtherSizeRateAndTwoLevels` covers a nested timeline of another size and rate and two levels of nesting.

## 4. The host interface

### 4.1 One module, no imports

The renderer is a Cargo workspace under `renderer/` (ADR-0029 item 2). Two builds come out of it, and the difference is the only reason there are two:

| Build | Target | Carries | Used by |
| --- | --- | --- | --- |
| **core module** | `wasm32-unknown-unknown`, plain exports (below) | the CPU path of §3 | the Go export through `wazero`; the browser worker as the CPU fallback |
| **GPU module** | `wasm32-unknown-unknown` with `wasm-bindgen` glue | the CPU path and the `wgpu` path of §3.8 | the browser worker, on WebGPU |

**Decision of this SDD (DS-1).** The core module is built without `wasm-bindgen` (the crate does not depend on it; the GPU module of stage D will, in its own crate): `wasm-bindgen` output imports a table of helper functions from its JavaScript glue, and ADR-0029 requires that the module the Go export runs has **no import except its memory**, so that it cannot reach a file, the network or the clock. The core module exports plain C-ABI functions, so both hosts load it without glue code (the browser with `WebAssembly.instantiate`, Go with `wazero`). The GPU module may import what `wgpu` needs from the browser, and runs only in the browser's worker, where the browser's own sandbox is the boundary.

The core module has these properties, each tested (§4.8):

1. It declares **no import**. `wasm-objdump -x` (or `wazero`'s `CompiledModule.ImportedFunctions()`) lists none.
2. It exports its memory as `memory` and the functions of §4.2, and nothing else.
3. It declares a **maximum** memory size (65,536 pages of 64 KiB would be 4 GiB; the module declares 16,384 pages, 1 GiB) and a panic aborts the call with the `unreachable` instruction, which the host turns into `render_trap` (§7).
4. It is single threaded, has no start function that runs code, and uses no `simd128` unless §3.9 rule 6 allows it.
5. It is a build artifact: reproducible (`scripts/build-renderer.sh`, whose `embed` mode writes `engine/renderhost/renderer.wasm` and its SHA-256 in `renderer.wasm.sha256`, and whose `check` mode fails when the embedded module is not the one the source builds), embedded in the Go binary with `go:embed`, served to the browser at `/assets/renderer-<sha256 first 12 hex>.wasm`, and checked by digest like every pinned input (ADR-0029, Consequences).

### 4.2 The exports

All numbers are `i32` or `u32` (WebAssembly `i32`), pointers are offsets into `memory`, and a length is in bytes. The host owns every buffer it passes; the module never keeps a pointer between calls, it copies what it needs to keep.

| Export | Signature | Meaning |
| --- | --- | --- |
| `vr_api_version` | `() -> u32` | `API_VERSION` of `renderer/stuarty-renderer/src/lib.rs`, 3 here (2 added the text exports below, 3 added `vr_put_lut`). A host refuses a module whose number it does not know (`unsupported_version`) before any other call |
| `vr_alloc` | `(len: u32) -> u32` | reserves `len` bytes, 8-aligned, and returns the offset; 0 when the module is out of memory (`out_of_memory`). `len` 0 returns a valid non-null offset |
| `vr_free` | `(ptr: u32, len: u32)` | releases a block obtained from `vr_alloc` with the same `len` |
| `vr_init` | `(max_texture_bytes: u32, max_bitmap_bytes: u32) -> i32` | prepares an empty renderer with its two budgets (§7); a second call resets it. The status is 0 or a negative code of §4.5 |
| `vr_put_texture` | `(id_ptr: u32, id_len: u32, width: u32, height: u32, rgba_ptr: u32, opaque: u32) -> i32` | stores the pixels of a media layer under the item id (`id_len` at most 64 bytes of ASCII): `width x height x 4` bytes of premultiplied RGBA; `opaque` 1 when every alpha is 255. Replaces the texture of that id. The module copies the pixels |
| `vr_put_bitmap` | `(key_ptr: u32, key_len: u32, width: u32, height: u32, natural_w: f64, natural_h: f64, center_dx: f64, center_dy: f64, rgba_ptr: u32) -> i32` | stores a bitmap of the rasterizer (§2.5) under its 64-character key: `width x height x 4` bytes of straight alpha; the module premultiplies it once |
| `vr_put_lut` | `(id_ptr: u32, id_len: u32, size: u32, data_ptr: u32, data_len: u32) -> i32` | stores the lookup table of a `lut` asset under its id (1 to 64 ASCII bytes): `size` from 2 to 65 points on a side and `data_len` exactly `size^3 x 3 x 2` bytes, the little-endian 16-bit entries of `engine/lut` (0 is 0.0, 65535 is 1.0, red varying fastest); replaces the table of that id; at most 32 tables and 64 MiB are held (`limit_exceeded`, detail `render_luts`) |
| `vr_release` | `(id_ptr: u32, id_len: u32) -> i32` | forgets the texture, bitmap or lookup table with that id or key; an unknown one is not an error |
| `vr_prepare` | `(in_ptr: u32, in_len: u32) -> i32` | epic 55 (§4.2.2): reads a render input with exactly one layer and prepares that layer (steps 1 to 5 of SDD-004 §3.3, at opacity 1); the result block is read with the next three exports |
| `vr_prepared_ptr` / `vr_prepared_len` | `() -> u32` | the offset and the byte length of the block of the last `vr_prepare`, 0 when that layer draws nothing; valid until the next call |
| `vr_prepared_clear` | `()` | releases that block |
| `vr_put_prepared` | `(id_ptr: u32, id_len: u32, blk_ptr: u32, blk_len: u32) -> i32` | stores a block of `vr_prepare` as the texture of that id (§4.2.2); the module copies it |
| `vr_render` | `(in_ptr: u32, in_len: u32, out_ptr: u32, out_len: u32) -> i32` | reads the render input of §2 (UTF-8 JSON, `in_len` at most 4 MiB), draws it, and writes `width x height x 4` bytes at `out_ptr`; `out_len` must be exactly that. The status is 0 or a negative code of §4.5 |
| `vr_error` | `(buf_ptr: u32, buf_len: u32) -> u32` | copies the message of the last failed call (UTF-8, truncated to `buf_len`, no personal data: ids and keys only) and returns its full length |
| `vr_text_font_put` | `(hash_ptr: u32, hash_len: u32, data_ptr: u32, data_len: u32) -> i32` | stores the bytes of a user font under its 64 lower-case hex SHA-256 for the text requests that name it (at most 16 MiB a file, 64 MiB in all, `limit_exceeded`); no bytes forget it |
| `vr_text_run` | `(req_ptr: u32, req_len: u32) -> i32` | draws or measures a shape, a text or a caption (the request of §4.2.1, `req_len` at most 16 MiB); the result is read with the next two exports |
| `vr_text_validate_font` | `(data_ptr: u32, data_len: u32) -> i32` | validates a user font file (SDD-007 §7.7): size, magic, table directory, the tables every font needs; a refusal is `font_invalid` and its reason and message are the result |
| `vr_text_result_ptr` / `vr_text_result_len` | `() -> u32` | the offset and the byte length of the result of the last `vr_text_*` call, valid until the next call |
| `vr_text_result_clear` | `()` | releases that result |

#### 4.2.1 The text request

The rasterizer of SDD-004 §5 is `renderer/stuarty-renderer-text`, run by the module (stage E, TASK-0220). A request is little-endian: `u8` mode (0 draw, 1 measure), `u8` kind (0 shape, 1 text, 2 caption), `f64` pixel scale, then for a shape its kind (0 rect, 1 rounded rect, 2 ellipse), `f64` width, height, radius and stroke width and the fill and stroke colours (4 bytes, straight RGBA); for a text or caption a count of document font families, each `str` name and a count of files of `u32` weight and `str` digest, then the style (`str` family, `u32` weight, `f64` size, colour, stroke colour, `f64` stroke width, shadow colour, `f64` offset x, offset y and blur, an optional box colour as a flag byte and 4 bytes, `f64` padding and radius, `u8` alignment 0 left, 1 centre or 2 right, `f64` line height and maximum width, the upper-case flag, the highlight flag, the highlight colour, an optional highlight box colour) and the content: for a text a `str`, for a caption a count of `str` words, a count of `u32` breaks and `u32` active word plus one. A `str` is a `u32` length and UTF-8 bytes; a count is at most 65,536.

The result of a successful call is `u32` width and height, `f64` natural width and height and centre offset x and y, a `u32` count of word boxes with four `f64` each, then `width x height x 4` bytes of straight-alpha RGBA; a measure has width and height 0 and no pixels. A failure has a status of §4.5 and `vr_error`; `font_unknown` also leaves as the result the family, the weight, the weights that exist (a flag, a count and the numbers) and the digest not stored (a flag and a `str`), and `font_invalid` the reason and the message. A user font a request names that the module does not hold is `font_needed` with the digest as the detail: the host reads the file, calls `vr_text_font_put` and repeats the request, or reports `E_FONT_UNKNOWN` itself when the workspace has no such file. The module keeps parsed fonts by digest, so a host that wants a font to be unknown to a document whose workspace lacks it checks its own source first (`engine/rasterize` does). `engine/renderhost` is the Go host of these exports (`PutFont`, `RunText`, `ValidateFont`); `engine/rasterize` encodes the requests.

#### 4.2.2 The prepared layer (epic 55)

A layer is drawn in two parts: its **preparation** (SDD-004 §3.3 steps 1 to 5: the shrink of a bitmap, the chroma key, the tone table, the colour matrix, the grade, the lookup table, the filter, the blur, the filters of §3.11.3, the flip, the effects, the shadow and the outline) depends only on its pixels, its size and its looks; its **draw** (the sample or copy, the mask, the opacity and the blend mode) depends on where it is. `vr_prepare` runs the first part on a render input of one layer, whatever its opacity (it is prepared as if it were 1) and its position, and returns a block, little endian: `u32` width, `u32` height, `u32` flags (bit 0: every alpha is 255), `u32` 0, then `f64` grown width and grown height (the size of the rectangle that draws the prepared pixels, in output pixels), `f64` offset x and offset y (what a bitmap's metrics move the centre by, 0 for a media layer) and `f64` scale (the `output.scale` it was prepared at), then `width x height x 4` bytes of premultiplied RGBA. A layer that draws nothing (a zero size, no pixels) gives an empty block. The layer needs its texture or bitmap, and the tables it names, stored as for `vr_render`; the render input it comes in is validated as for `vr_render`.

`vr_put_prepared` stores the block as the texture of the id. A `media` layer of a render input whose `item` is that id is a **prepared layer**: it is drawn with no look, no blur, no flip, no effect, no shadow and no outline (they are in the pixels), with its centre at `(cx - offset x, cy - offset y) x scale`, the grown size and its `rotation`, and with its `mask`, `transition` wipe, `opacity`, `blend`, `clip` and `stencil` as any layer. The layer must be the one that was prepared, apart from those members and its position; the render input names the same `scale`, or the call is `invalid_plan` with the detail `prepared_scale`. The result is the same pixels as the layer drawn whole (`tests/prepared.rs` draws every golden case of `testdata/image-ops/` that way, whole, in six bands and moved; `engine/renderhost/prepared_test.go` does it on other instances of the module under `wazero`). Textures count against the budget of `vr_init` as any texture does. The browser host does not use these exports and the version number does not change: a module with more exports than a host uses is still the module that host knows.

`f64` parameters are part of the export signature. A host that cannot pass a double (none of ours) passes them through a block written with `vr_alloc`; that form is not defined because no host needs it.

Textures and bitmaps live in the module's memory between calls, so a preview that scrubs over the same layers uploads each frame of video once (§5) and each bitmap once per key. They are evicted by the host with `vr_release`, or all at once by `vr_init`. The module itself never evicts: it counts the bytes against the two budgets of `vr_init` and refuses a `put` that would pass them with `limit_exceeded` (§7), so the host decides what to drop.

### 4.3 One render, in order

A host renders a frame with this sequence, and no other is supported:

1. `vr_alloc` the output block of `output.width x output.height x 4` bytes, once, and reuse it while the size is the same.
2. For every media layer of the plan not already uploaded, `vr_alloc` the pixels, write them, `vr_put_texture`, `vr_free` (§5).
3. For every bitmap layer whose key is not already stored, the same with `vr_put_bitmap` (§2.5), and for every lookup table a layer names (`lut`) that is not already stored, `vr_put_lut` (the Go export uploads and releases the tables of a frame like its textures).
4. `vr_alloc` the input, write the render input, `vr_render`, `vr_free` the input.
5. Read the frame from the output block. A status other than 0 means the output block holds nothing to use.

A `vr_render` that needs a texture or a bitmap the module does not hold fails with `missing_texture` (§7) and names the item or the key in `vr_error`; the host uploads it and calls again. `vr_render` never blocks, never retries and never reads anything the host did not put in memory.

### 4.4 Memory and ownership rules

1. The host owns every block. The module reads `in_ptr`, `rgba_ptr` and the id and key blocks during the call only, and writes `out_ptr` during `vr_render` only.
2. A block is valid from `vr_alloc` to `vr_free`; a pointer used after `vr_free`, or a length that does not match, is a host bug and is not defended against by the module beyond staying inside its own memory (every access is bounds-checked by WebAssembly; the module is built with overflow checks on in release).
3. The memory may grow during `vr_alloc`, `vr_put_texture` and `vr_put_bitmap`, which moves nothing but invalidates a host's `ArrayBuffer` view in the browser (`memory.buffer` is a new object): the host re-reads `memory.buffer` after each call that may allocate. `wazero`'s `Memory().Read` has no such trap.
4. The module's peak memory for one render is the stored textures and bitmaps, plus the scratch of one layer at the size of §3.4 and §3.5, plus the output block; the budgets of §7 keep that below the declared maximum.

### 4.5 Status codes

`vr_init`, `vr_put_*`, `vr_release`, `vr_render` and `vr_text_*` return `0` for success and a negative number for a failure. The numbers are the `reason` strings of §7, in this order; the mapping is part of the API version.

| Status | `reason` |
| --- | --- |
| -1 | `invalid_plan` |
| -2 | `unsupported_version` |
| -3 | `unsupported_layer` |
| -4 | `missing_texture` |
| -5 | `limit_exceeded` |
| -6 | `out_of_memory` |
| -7 | `invalid_argument` |
| -8 | `invalid_spec` |
| -9 | `font_unknown` |
| -10 | `font_invalid` |
| -11 | `font_needed` |
| -12 | `text_failed` |

A trap (`unreachable`, an out-of-bounds access the module's own checks missed, a stack overflow) does not come back as a status: the host sees it as a failed call on the instance and reports `render_trap`. After a trap the instance is discarded and a new one is created; its stored textures are gone and the host uploads again.

### 4.6 The Go host

The Go export runs the core module through `wazero` (pure Go, no cgo; approved for this use by the author, AGENTS.md rule 9) in a package `engine/renderhost`. The package is the only place that knows the exports of §4.2.

```go
package renderhost

// Host runs one instance of the core module. It is not safe for concurrent use.
type Host struct{ /* wazero module, memory, the output block */ }

// New instantiates the embedded module (compiled once per process; the digest is checked, an
// import or a missing export refuses it) and checks that vr_api_version is 3.
func New(ctx context.Context, cfg Config) (*Host, error)

// Output is the frame to write; Y1 > Y0 names the rows of a band.
type Output struct { Width, Height int; Scale float64; Y0, Y1 int }

// RenderInput is Output and the plan.FramePlan to draw; its JSON is the render input of section 2.1,
// with bitmap_key added to the bitmap layers.
type RenderInput struct { Output Output; Plan plan.FramePlan }

// PutTexture uploads the premultiplied RGBA pixels of a media layer.
func (h *Host) PutTexture(ctx context.Context, item string, w, h int, rgba []byte, opaque bool) error
// PutBitmap uploads a bitmap of the rasterizer under its key.
func (h *Host) PutBitmap(ctx context.Context, key string, bm bitmap.Bitmap) error
func (h *Host) Release(ctx context.Context, id string) error
// Render draws the frame into dst (RGBA, no stride padding, exactly output.width x output.height);
// with a band only those rows of dst are written.
func (h *Host) Render(ctx context.Context, in RenderInput, dst *image.RGBA) error
func (h *Host) Close(ctx context.Context) error
```

- **Sandbox.** The `wazero` runtime is configured with no WASI, no file system, no clock (no `WithSysWalltime`, `WithSysNanotime` or `WithSysNanosleep`), no random source, no environment and no arguments. The module has no import to satisfy, so there is no host function module either. This is what the threat row that ADR-0029 adds to SDD-007 relies on, and its test is in §4.8.
- **Limits.** `WithMemoryLimitPages(16384)`, the same as the module's declared maximum; `WithCloseOnContextDone(true)`, so cancelling an export stops a render that is in progress (the export's own cancel, SDD-004 §8, reaches it). The instances that draw text (`Config.Uncancellable`, `engine/rasterize`) use a second runtime with `WithCloseOnContextDone(false)`: the check that makes a call stoppable sits on every loop of the module and made a caption bitmap 5 times slower (57 ms against 11.5 ms on the machine of TASK-0220), and a text request is bounded by its limits (a bitmap of at most `MAX_PIXELS`, a request of at most 16 MiB, at most 65,536 words or families); `TestSecurity_T36_TextRequestsSurviveHostileInput` mutates requests and requires the instance to stay usable. A frame has no deadline of its own: the export's cancel and its overall deadline (SDD-007 §6.5) reach it through the context.
- **Bands.** The compositor of SDD-004 §12 splits a frame into bands that run in goroutines. The Rust module is single threaded, so the host runs one instance per band and gives each a `vr_render` of the rows it owns: the render input of a band carries `output.y0` and `output.y1` (§2.1; absent for the whole frame) and the module draws only those rows of the full output, which is why a band never changes a byte (§3.9 rule 5). The number of instances is the number of bands of the export, at most 8.
- **Errors.** A non-zero status becomes a `*renderhost.Error{Reason, Detail}`; the export maps it to `E_EXPORT_FAILED` with `details.reason` (§7). A trap discards the instance, and the export fails, it does not retry on a new instance (a trap on the same input is deterministic).
- **Frames in.** The export keeps decoding with FFmpeg (§5.2) and hands each decoded frame to `PutTexture`; the compositor stays in Go until stage C's gate passes (ADR-0029 item 8).

### 4.7 The browser worker protocol

The preview owns one dedicated module worker, `renderWorker.ts` in `web/resources/js/editor/preview/`, created with `new Worker(url, {type: "module"})` from a same-origin script (SDD-007 §8.5.5: no inline worker, no `blob:` URL; the CSP of the editor must allow `worker-src 'self'`). The main thread never calls the module: it posts messages and receives frames.

Messages from the main thread to the worker:

| `type` | Fields | Effect |
| --- | --- | --- |
| `init` | `module` (the `/assets/renderer-<digest>.wasm` URL), `canvas` (an `OffscreenCanvas`, transferred, for the GPU path), `prefer` (`"gpu"` or `"cpu"`), `budgets` | fetches and instantiates the module, checks `vr_api_version`, chooses the path, calls `vr_init` |
| `texture` | `id`, `frame` (an `ImageBitmap` or a `VideoFrame`, transferred), `opaque` | uploads a media layer's pixels (§5.1); the worker closes the transferred object after the copy |
| `bitmap` | `key`, `bitmap` (an `ImageBitmap`), `natural`, `center` | uploads a bitmap |
| `release` | `ids` | `vr_release` for each |
| `render` | `seq`, `input` (the render input of §2) | draws a frame |
| `dispose` | none | frees the module, closes every `VideoFrame` still held, ends the worker |

Messages from the worker:

| `type` | Fields | Meaning |
| --- | --- | --- |
| `ready` | `apiVersion`, `path` (`"gpu"` or `"cpu"`) | the module is initialised; `path` is what was chosen |
| `frame` | `seq`, `bitmap` (an `ImageBitmap`, transferred) or, on the GPU path, `presented: true` | the frame of `seq` is drawn; the CPU path returns the pixels, the GPU path draws on the canvas it was given |
| `error` | `seq` (absent for `init`), `reason`, `detail` | a failure with a `reason` of §7; for `missing_texture`, `detail` names the item or key and the main thread uploads it and posts the same `render` again |

Rules:

1. **At most one render in flight.** The main thread does not post a `render` while one is pending; it keeps the newest request and posts it when the answer comes. A scrub therefore costs one frame of latency, not a queue.
2. **Latest wins.** A `frame` whose `seq` is older than the newest `render` posted is dropped by the main thread and its bitmap closed.
3. **Every `VideoFrame` is closed** by the worker after its pixels are copied or its texture is created (§5.1), on the error paths too. A leak is a failed test (§4.8).
4. **The worker has no network access of its own**: the module URL and the media come through messages and `fetch` calls the main thread allows by CSP (`connect-src 'self'`).
5. **WebGPU loss.** On `device lost` the worker answers every pending and following `render` with `error` and `reason: "device_lost"`, then tries once to re-create the device; if that fails it switches to the CPU path and sends `ready` with `path: "cpu"`. The main thread then resends the textures it still has.
6. **Fallback.** With no adapter (`navigator.gpu` missing or `requestAdapter()` null) or `prefer: "cpu"`, the worker uses the core module and draws on a 2D canvas (`ImageData`), the same pixels as the export's CPU path.

#### 4.7.1 What is implemented (epic 26, 2026-11-19)

| Part | State |
| --- | --- |
| §5.3 decode worker, §6 chunk index consumer (`preview/decode/`) | implemented, used by both executors |
| §2.4 expansion of a window to the render input of a frame, in TypeScript (`render/frameInput.ts`) | implemented; tested for the names and shapes the module accepts (a layer that uses every field is read by the real module). The byte-for-byte twin of Go's `plan.Frame` is not written: the plan has no Go `WindowPlan.Frame`, and the golden comparison of `preview-export-parity` is what holds the two |
| §4.7 render worker (`render/renderWorker.ts`, logic in `renderService.ts`), its protocol (`protocol.ts`) and the main-thread client (`client.ts`) | implemented on the CPU path |
| the executor that feeds the worker behind `PreviewExecutor` (`RustExecutor.ts`), the choice by feature detection and a setting (`executorChoice.ts`, `ExecutorSwitch.ts`) | implemented |
| §3.12 groups: a `group` layer of a window drawn live by the worker on the CPU path (`render/groups.ts`, the `surfaces` of the `render` message), through an intermediate surface | implemented (epic 52); see "Groups in the worker" below |
| §3.12 groups in the PixiJS fallback | drawn approximately, through a render texture per group (epic 52); the executor says so on its diagnostic line |
| §4.7 GPU path (`wasm-bindgen` GPU module, `OffscreenCanvas` presented by `wgpu`) | **not implemented**; there is no GPU module for the browser. `prefer: "gpu"` is answered with `path: "cpu"` |

Differences from the protocol above, all of them of the CPU path: `init` carries the SHA-256 the module must have (`digest`, from `renderer.wasm.sha256`), and a file that differs is refused with `module_digest` (a fetch or compile failure is `module_load`); `texture` carries an optional `rect`, the crop of the layer, in the pixels of the frame, and an optional `size`, the size to deliver it at: like the Go host for a single frame (`target` in `engine/export/media.go`, §5.1 item 4), a layer that is not rotated at that frame is delivered at its drawn size, rounded, and any other at its cropped size, so the effects that work on the layer's own pixels (grain, the channel split) see the image the export's do (they were 0.90 of luma SSIM from the export's frame with the cropped size); after a trap the worker discards the instance and starts a new one from the checked bytes (§4.5), and the page uploads again; the module is served under the content-hashed name the build gives it (`renderer-<hash>.wasm`), not under the first 12 hex digits of its SHA-256 (§4.1 item 5), and its digest is checked in the worker; a `lut` message uploads a lookup table (`vr_put_lut`); the worker draws on the CPU path and answers `frame` with an `ImageBitmap`, which the page draws on a 2D canvas (so the preview reads its pixels for the eyedropper); the `OffscreenCanvas` of `init` arrives with the GPU module. The worker needs the CSP source `'wasm-unsafe-eval'` (SDD-007 §8.5.5), because a browser compiles WebAssembly only where the policy allows it.

**Groups in the worker (epic 52).** The module never draws a group (§3.12), so the host does, as the Go host does: the page expands a `group` layer of the window (the layers of `group.layers` carry the series of the parent's frames, so a nested layer at the playhead frame is read at that frame) into a tree, and the `render` message carries the `surfaces` the frame needs, `[{id, input}]`, each the render input of the nested frame of a group (`{width, height: group size, transparent: true, background: "#000000"}`, output size `sw` by `sh`, output scale `k` of §3.12 steps 1 and 2), the inner ones first. The worker draws each with the module, in order, and stores the pixels as the texture `id` (premultiplied as drawn, not opaque), then draws the frame, whose input holds the group as the media layer of §3.12 step 4. A surface is drawn again for every frame that holds its group (a nested layer moves), under a short id of the page (`~<n>`, one per path of group items: the nested ids of two uses of one timeline are the same, and an id is at most 64 bytes), and released when its group leaves the frame. The surfaces alive at once are those of the groups of the current frame: one per group, so as many as the depth of §3.12 along one path and one more for each group beside another. The pool is the module's texture table, bounded by its `textureBytes` budget (256 MiB by default): a surface drawn again under its id replaces its pixels (`renderService.test.ts` draws a surface of 4096 by 4096 pixels five times), and a frame whose surfaces and textures together pass the budget is answered with `limit_exceeded`, as any texture that passes it. Nested media are delivered at the size of the surface scale `k`, not the output scale. No `ViewFrame` request is made while the timeline plays: the paused frame still comes from the server (SDD-004 §3.16). What the browser still does not do: draw a group on the GPU path (there is none), and a group deeper than the limit of §3.12 (the layer is left out).

### 4.8 Acceptance of this section

- `TestCoreModuleHasNoImports` (Go): the embedded module has zero imported functions, memories, tables and globals, exports exactly `memory` and the functions of §4.2, and `vr_api_version` returns 3.
- `TestSecurity_T36_RenderHostIsSandboxed` (Go): a module compiled for the test that tries to import `wasi_snapshot_preview1` or `env` fails to instantiate, and the production configuration has no clock or file system (the test inspects the `wazero` configuration the host builds).
- `TestRenderHostCancels` (Go): cancelling the context of a long render returns `context.Canceled` within 1 s and leaves no goroutine.
- A Vitest and a Playwright test for the worker: a render answers `frame` with the `seq` asked; two renders posted in a row draw only the newest; the `VideoFrame` count reported by the worker is 0 after 500 renders with uploads (the leak check of stage B's gate); `missing_texture` is answered by an upload and a second `render`, not by a black frame.

### 4.9 The native GPU host (stage F)

`stuarty-render` is the binary of `renderer/stuarty-renderer-gpu` (`src/bin/stuarty-render.rs`). It draws what `GpuRenderer` draws (§3.8) for a Go export that asked for `gpu`, in a child process because a graphics driver is code that must not live in the owner process (ADR-0029 item 4, SDD-007 §6.10). The Go side is `engine/gpuhost`; the protocol is `renderer/stuarty-renderer-gpu/src/proto.rs` and nothing else knows it.

- **Command line.** `stuarty-render --version` prints `stuarty-render <version>`; `--probe` prints the adapter (name, backend, device type) and exits 0, or exits 3 when the machine has no adapter; no argument serves the protocol on standard input and output; anything else exits 2. It takes no path, no URL and no environment variable, reads no file and opens no connection.
- **Protocol.** Little endian. A request is `u8` kind, `u32` payload length, the payload; a reply is `u8` status, `u32` payload length, the payload. Kinds: 0 init (`u32` texture budget, `u32` bitmap budget, 0 for the defaults of §7.2; first and once; the reply is the adapter text), 1 texture (`u8` id length, id, `u32` w, `u32` h, `u8` opaque, premultiplied RGBA8), 2 bitmap (key, `u32` w, `u32` h, four `f64`, straight RGBA8), 3 release (id), 5 lut (`u8` id length, the asset id, `u32` size, `size^3 x 3` little-endian `u16` entries), 4 render (the render input of §2.1; the reply is the frame, `width x height x 4` bytes; `y0` and `y1` are accepted, the Go host sends none). Status 0 is success, 1 to 7 are the reasons of §7.1 with the detail as the payload (the child goes on), 8 is a failure of the device and 9 a failure of the protocol (the child answers and ends). End of input at a message boundary ends the child with exit 0.
- **Limits.** A request payload is at most `8192 x 8192 x 4 + 4096` bytes and is read with `take`, so a lying length allocates nothing; a larger length, a short payload, an unknown kind, a second init and a request before the init are protocol failures. Sizes, layers and the plan are validated by the same decoder as the CPU path, and the budgets of the init bound the stored pixels.
- **Choice.** The Go export uses it only when `STUARTY_EXPORT_RENDERER=gpu`, a path is configured, the runner has no `bwrap` wrapper (which hides the devices) and the child answered the init; otherwise the CPU path (§4.6) draws, and the CPU path stays the default and the truth.
- **Packaging.** Every archive and installer that carries `ffmpeg` carries `stuarty-render` next to the server executable (or in `libexec/stuarty/`, like FFmpeg); built by `cargo build --locked --release --bin stuarty-render` in `renderer/`.

Acceptance: `renderer/stuarty-renderer-gpu/tests/proto.rs` (the protocol against the CPU path, the limits, the refusals and the binary on its pipes), `TestSecurity_T37_*` in `engine/gpuhost` and `engine/ffmpeg`, `TestGPUHostIsHeldToTheCPUPath` (SSIM 0.99 on the reference scenes through the child, with the adapter in the log) and `TestExportWithTheGPUHost` in `engine/export`.

## 5. Frames from the hosts

The renderer draws what it is given; it never decodes video. Each host gets the pixels of a media layer in its own way and hands them over as a **texture** under the item id (§4.2, `vr_put_texture`). This section fixes what a texture is, and how each host makes one.

### 5.1 What a texture is

1. **Cropped already.** The texture is exactly the region the layer's `crop` names, in source pixels. The renderer draws the whole texture into the layer's rectangle and ignores `crop` (it is in the plan for the host, SDD-004 §6 "Pre-scaling"). A host that cannot crop while decoding crops by copying the rectangle.
2. **Premultiplied RGBA8**, with `opaque` set when every alpha is 255. A video proxy has no alpha, so its texture is opaque (the WebM proxy of a `has_alpha` asset keeps it, but it is played by the `<video>` element of SDD-004 §10 and is not decoded by WebCodecs, whose index it does not have); the frame of an asset with `has_alpha` (SDD-011 §4.3.5, the Go export only) is premultiplied by the host, as SDD-004 §6 says.
3. **Non-linear sRGB-encoded values**, converted from the normalized media's BT.709 limited-range YUV by the host (§5.2 and §5.3). The renderer never converts a colour.
4. **Its size is the delivered size.** A constant, unrotated layer may be delivered already scaled to its rounded drawn size (the copy path of §3.6); any other is delivered at the cropped native size and the renderer samples it. The host decides which, as `scanLayers` does in the Go export (SDD-004 §6), and says nothing to the renderer: the texture's own size is the signal.
5. **One texture per item**, replaced at each frame (`vr_put_texture` with the same id). Two layers that show the same asset are two items and have two textures.
6. **Images** (a still or one frame of an animated image) are decoded once by the host from the normalized PNG (Go export) or from the image proxy with `createImageBitmap` (browser), cropped, premultiplied and uploaded once; the texture stays until the layer's item leaves the window or its crop changes.

### 5.2 The Go export: raw frames from FFmpeg

Nothing changes in how frames are decoded. `engine/export`'s `frameSource` starts one FFmpeg process per active clip with the command of SDD-004 §6 (`crop`, `scale=...flags=bicubic+accurate_rnd+full_chroma_int`, `format=rgba`, `-f rawvideo pipe:1`, with `in_color_matrix=bt709:in_range=tv` explicit), reads `delivered_w x delivered_h x 4` bytes per frame and today passes them to the Go compositor as an `Image`. With the Rust renderer it passes the same bytes to `Host.PutTexture(item, w, h, rgba, opaque)` (§4.6). Because the bytes are the ones the Go compositor gets today, stage C's byte-for-byte gate (§3.10) tests the renderer and not the decoder. Rules:

- Frames are read in order, three ahead, with the restart rules of SDD-004 §6 (a request more than 120 frames ahead or an earlier frame restarts the process). The host, not the renderer, repeats the last frame past the end of the media and reuses a frame for speeds below 1.
- A frame is uploaded **once per render** for its layer. The host does not keep textures of frames that are not drawn: after the frame's `vr_render` it calls `vr_release` for the layers that left the frame, so the texture budget (§7) is the layers of one frame.
- Pixel buffers are the host's; `PutTexture` copies (§4.4), so the decoder reuses its buffers at once.
- The alpha of a `has_alpha` asset: FFmpeg delivers `rgba`; the host premultiplies each frame as it arrives with the `div255` of §3.2 before `PutTexture` (not inside the module), so the module sees only premultiplied textures.

### 5.3 The browser: WebCodecs

The preview worker decodes the video layers of the window from the proxies (ADR-0029 item 5) using the chunk index of §6. `<video>` stays as the fallback of §5.3.5.

1. **Configure.** For each asset with a video layer the decoder worker fetches `GET .../media/{asset_id}/chunks` once (cached with the proxy's rules), then checks `VideoDecoder.isConfigSupported({codec, codedWidth, codedHeight})` with the index's `codec`, `coded_width` and `coded_height`. If it is supported, one `VideoDecoder` per asset is configured with the same values; otherwise the asset is decoded by the fallback.
2. **Which sample.** The plan window gives `source_seconds[t]` for each frame (SDD-004 §10), the time in the proxy at which the frame to show starts. The sample to show is the one with the largest `timestamps_us[i]` not above `round(source_seconds x 1,000,000)` plus half a sample duration (0.5 frame of the asset's rate); it is found by binary search on `timestamps_us`, which is sorted because the proxy has no B-frames.
3. **Seek (paused, scrubbing, a window change).** Find the last key frame `k <= i` in `keyframes` (binary search). If the decoder is already positioned so that `i` follows its last decoded sample by at most 4 samples and no key frame lies between, keep decoding forward. Otherwise: `reset()`, `configure()` again, then fetch the bytes of samples `k` to `i` with one `Range` request on the proxy (`offsets[k]` to `offsets[i] + sizes[i] - 1`), cut them with the index into `EncodedVideoChunk`s (`type: "key"` for `k`, `"delta"` for the others, `timestamp` and `data` as the index says) and `decode()` them in order, then `flush()`. Output frames whose timestamp is not that of sample `i` are closed on arrival; the frame of sample `i` is the result. At most `GOP - 1` samples are decoded and thrown away (SDD-011 §4.4: a key frame every half second, 15 samples at 30 fps), which is what keeps a scrub inside the budget of NFR-003.
4. **Play.** Forward playback keeps decoding after the last sample, in order, at most 4 decoded `VideoFrame`s ahead of the one being shown per layer, and respects `decoder.decodeQueueSize <= 8` before queueing more chunks. The audio clock decides which frame is shown (SDD-012 §11.2 item 4): frames older than the target are closed, not drawn. At the end of the media the last frame stays.
5. **Unsupported, or an error.** When `isConfigSupported` answers false, when the decoder's `error` callback fires, or when a `Range` fetch fails, the worker marks the asset as `fallback`, closes its decoder and every `VideoFrame` of it, and tells the main thread, which draws that asset's layers from a `<video>` element as the earlier preview does (`videoPool.ts`) and sends the picture to the renderer through `createImageBitmap(video)` as a texture. A fallback is per asset and lasts until the page is reloaded; it is shown by a stats counter (`stats.fallbackAssets`), not by a toast.
6. **From a `VideoFrame` to a texture.**
   - **CPU path.** The worker copies the frame's `rect` (the layer's `crop`, rounded to whole pixels) with `VideoFrame.copyTo(buffer, {format: "RGBA", rect, colorSpace: "srgb"})`, a conversion done by the browser from the colour tags of the proxy (BT.709 limited range, tagged by the ingest, SDD-011 §4.3), and uploads the bytes with `texture` (§4.7). The frame has no alpha, so `opaque` is 1.
   - **A crop that starts on an odd pixel.** `copyTo` converts 4:2:0 in blocks of 2 x 2 and returns such a `rect` shifted by two pixels (the golden case `crop` was 5 output pixels off the export). The worker copies the even rectangle that holds the crop (`render/pixels.ts`, `evenAround`) and cuts the crop out of it.
   - **GPU path.** The worker uses the frame as an external texture or copies it to a GPU texture (`importExternalTexture`, or `copyExternalImageToTexture` for the cropped rectangle) and draws with it in the same task.
   - **Close.** `frame.close()` is called as soon as the pixels are copied (CPU path) or, on the GPU path, in the same task that submitted the commands that use it (an external texture expires at the end of the task). A frame is never kept across a message without being counted in `stats.openFrames`, and a `render` never starts with a frame whose `close()` was already called: that is the black texture of P2-QA-003.
7. **Tolerance.** The browser's conversion from YUV to RGB is not the one FFmpeg does, so the preview's frames are in the tolerance class of §3.8 and ADR-0003 item 6, never byte-equal to the export's. Truth on pause is unchanged: the paused frame is the export's frame from `ViewFrame` (SDD-004 §9, SDD-012 §11.3).

### 5.4 The two hosts side by side

| | Go export | Browser worker |
| --- | --- | --- |
| Where the pixels come from | the FFmpeg process of SDD-004 §6, `rgba` pipe | the `VideoDecoder` on the proxy, or `<video>` as the fallback |
| Which frame | `source_frame` of the plan | `source_seconds` of the window, matched to `timestamps_us` |
| Crop | the decoder's `crop` filter | `VideoFrame.copyTo` `rect`, or a texture sub-rectangle |
| Colour conversion | FFmpeg, `bt709` limited explicit | the browser, from the colour tags |
| Alpha | host premultiplies | none (a proxy is opaque) |
| Equality required | byte for byte with today's compositor | the tolerance of §3.8 against the CPU path |
| Texture lifetime | one render | until the item leaves the window or the frame is replaced |
| Release | `vr_release` for layers that left | `frame.close()` at once, `release` message for evicted items |

### 5.5 Acceptance of this section

- Go: `TestHostFramesEqualCompositorFrames`: for every golden case, the textures handed to the renderer host are byte-equal to the `Image`s the Go compositor receives today (a comparison of the two inputs, before any drawing).
- Browser (Playwright, three browsers): `webcodecs-seek-is-frame-exact` seeks to 500 random frames of each reference clip and checks that the timestamp of the decoded frame is the one the plan asks for; `webcodecs-no-leak` seeks to 500 random frames and then plays, and checks that the peak of `stats.openFrames` (`peakOpenFrames`) and its value at rest never exceed the cache bound (client cache 12 + worker read-ahead 4 + 1 frame in transit = 17), so the count does not grow with the number of seeks; `webcodecs-fallback` forces `isConfigSupported` to false for one asset and checks that its layers draw through `<video>` and the others through WebCodecs.
- The gate of stage B: P2-QA-003 (black texture) does not appear in 500 repeated cases, and NFR-003 and NFR-004 hold on a quiet machine (ADR-0029, Stages).

## 6. The proxy chunk index

The engine writes `chunks-1.json` next to every video proxy (SDD-011 §4.4.1, `ingest.ReadChunkIndex`, tasks TASK-0195 to TASK-0197) and the service serves it at `GET /api/v1/projects/{id}/media/{asset_id}/chunks` (SDD-003 §6.5). SDD-011 §4.4.1 is where the file is produced and measured; this section is the contract of the consumer, the WebCodecs worker of §5.3, and the schema both sides test against.

### 6.1 The format, as a schema

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "title": "ChunkIndex",
  "type": "object",
  "additionalProperties": false,
  "required": ["version", "codec", "coded_width", "coded_height", "duration_us", "offsets", "sizes", "timestamps_us", "keyframes"],
  "properties": {
    "version": {"const": 1},
    "codec": {"type": "string", "pattern": "^vp09(\\.[0-9]{2}){8}$"},
    "coded_width": {"type": "integer", "minimum": 1, "maximum": 4096},
    "coded_height": {"type": "integer", "minimum": 1, "maximum": 4096},
    "duration_us": {"type": "integer", "minimum": 0},
    "offsets": {"type": "array", "items": {"type": "integer", "minimum": 1}},
    "sizes": {"type": "array", "items": {"type": "integer", "minimum": 1}},
    "timestamps_us": {"type": "array", "items": {"type": "integer", "minimum": 0}},
    "keyframes": {"type": "array", "items": {"type": "integer", "minimum": 0}}
  }
}
```

The schema is checked by `TestChunkIndexSchema` (Go, with the JSON Schema validator the contract tests already use): the index of every proxy of the ingest fixtures validates, and an index with an extra field, a `codec` of another family or a negative size does not. Beyond the schema, a consumer relies on these invariants, which `ingest.ReadChunkIndex` guarantees and the consumer verifies before using an index (§6.3):

1. `offsets`, `sizes` and `timestamps_us` have the same length `N`, which is the number of video samples of the proxy; `keyframes` is strictly increasing, every entry is below `N`, and `keyframes[0] == 0`.
2. `timestamps_us` is non-decreasing and `timestamps_us[0]` is 0 for the proxies of the ingest (the first sample is shown at the start). Decode order equals presentation order because the proxy is encoded without B-frames (`-lag-in-frames 0`, SDD-011 §4.3.2).
3. Every range `[offsets[i], offsets[i] + sizes[i])` lies inside the proxy file and holds exactly the bytes of sample `i`. Nothing else is promised about layout: the ranges need not be in order or contiguous.
4. `codec` is the WebCodecs codec string read from the `vpcC` box: `vp09.<profile>.<level>.<bit depth>.<chroma>.<colour primaries>.<transfer>.<matrix>.<full range>`, two decimal digits each.
5. `duration_us` is the duration of the video track; it may differ from the container duration by the audio track's length, and the editor uses the plan's `end_frame`, not this, for the project.

### 6.2 Versions

`version` is 1. A change that would make a v1 consumer decode wrongly (a different timestamp base, a different sample order, a codec that needs a `description`) is a new version and a new artifact profile (`chunks-2.json`, SDD-011 §6.3) written beside the old one. A new optional field that no v1 consumer needs is not a new version, but the schema above has `additionalProperties: false`, so adding one means changing the schema in the same pull request and the consumers' tests with it. The consumer refuses an index whose `version` it does not know and treats that asset as `fallback` (§5.3.5).

H.264 and other codecs are not in version 1: the proxy is VP9, and a codec that needs a `description` (the `avcC` of H.264) gets a `description` field in a later version, base64 encoded, with the codec string of its own registration.

### 6.3 What the consumer does with it

1. Fetch it once per asset with the cache rules of the proxy (SDD-003 §6.5: `ETag` `chunks-<hash16>-<profile>`, `private, max-age=86400`). The worker keeps the parsed index while the asset has a layer in a window or a decoder, and drops it with the asset (§5.3).
2. Validate it as §6.1 lists: schema (a small hand-written check in the worker, not a schema library), the equal lengths, the sorted `keyframes`, and, once the proxy is known, that `offsets[N-1] + sizes[N-1]` does not exceed the `Content-Length` the proxy answered. An index that fails is not used: the asset is `fallback`.
3. Use `offsets` and `sizes` for `Range` requests to the proxy (`GET .../proxy`, answered 206 by `http.ServeContent`, SDD-003 §6.5), `timestamps_us` for the sample to show (§5.3.2) and `keyframes` for the point to decode from (§5.3.3). The consumer never reads the MP4 boxes of the proxy.
4. Build `EncodedVideoChunk` with `type` `"key"` for the samples in `keyframes` and `"delta"` for the rest, `timestamp` from `timestamps_us`, `duration` omitted (the decoder does not need it) and `data` the range of bytes.

### 6.4 Old proxies and other assets

- An asset ingested before the chunk index existed (profile of the `proxy` artifact written by a version without it) has no `chunks` artifact, and the route answers `404 E_NOT_FOUND`. The worker treats a 404 as `fallback` for that asset: the preview works as before, through `<video>`. Importing the file again writes the index. A later change may queue a `profile_upgrade` ingest job for the missing `chunks` kind (SDD-011 §6.3 does this for artifacts that carry no facts) so that old assets get one without the user doing anything; that is not required by this SDD.
- An audio asset, an image asset and an asset without a video track have no index: 404, and nothing to decode.
- An asset whose proxy is not VP9 (none today) has none: `ErrNoChunkIndex`, no artifact, 404.

### 6.5 Acceptance of this section

- `TestChunkIndexSchema` and `TestProxyChunkIndexMatchesFFprobe` (`engine/ingest`, TASK-0197): the index validates against §6.1 and equals `ffprobe -show_packets` of the proxy on the pinned LGPL FFmpeg for the portrait, a 24 fps and a short landscape clip.
- A Vitest test of the worker's index check: each invariant of §6.1 broken in turn gives `fallback`; a valid index does not.
- The service test that opens kind `chunks` for a ready asset (`web/app/servicetest`, TASK-0196) answers `application/json` with the proxy's scope and cache rules.

## 7. Errors and limits

The renderer fails in a small, closed set of ways, each with a `reason` string, and it has a small, closed set of limits. No new error code is added to SDD-000 §8: a renderer failure reaches people and agents through the codes that already exist (`E_EXPORT_FAILED`, `E_LIMIT_EXCEEDED`, `E_CANCELLED`), with the `reason` in `details`. The preview never returns an API error: its failures are in the worker protocol (§4.7) and in the counters of the preview's `stats`.

### 7.1 Reasons

| `reason` | Status (§4.5) | Raised when | Where it surfaces |
| --- | --- | --- | --- |
| `invalid_plan` | -1 | the render input is not valid JSON of the shape of §2: an unknown field, a missing `opacity`, a matrix that is not 12 numbers, a number out of the double range, a bitmap layer without `bitmap`, a media layer with one | export: `E_EXPORT_FAILED`; preview: `error` message |
| `unsupported_version` | -2 | `version` of the render input, or `vr_api_version` of the module, is not one the other side knows | export: `E_EXPORT_FAILED`; preview: `error`, then the main thread reloads the module |
| `unsupported_layer` | -3 | `kind` is neither `media` nor `bitmap`, or a `BitmapSpec.kind` is not `text`, `caption` or `shape` (until stage E) | same |
| `missing_texture` | -4 | a layer needs a texture or bitmap the module does not hold (§4.3); `detail` names the item or key | export: the host uploads it and calls again, and only a second failure is an error; preview: the main thread uploads and posts the `render` again |
| `limit_exceeded` | -5 | a limit of §7.2 would be passed; `detail` names it | export: `E_LIMIT_EXCEEDED`; preview: `error` |
| `out_of_memory` | -6 | `vr_alloc` or a memory growth failed | §7.3 |
| `invalid_argument` | -7 | a pointer, length, size or id that does not match what the call needs: a texture whose `rgba` is shorter than `width x height x 4`, an id longer than 64 bytes, an `out_len` that is not the frame size, a `y1` not greater than `y0` | export: `E_EXPORT_FAILED` (a host bug); preview: `error` |
| `invalid_spec` | -8 | a text request names a spec the rasterizer cannot draw: a pixel scale outside (0, 8], a non-positive size, line height or width, a caption with no word, an active word out of range, an unknown kind | export: `E_EXPORT_FAILED`; the lint and the service measure it as 0 by 0 |
| `font_unknown` | -9 | the family or the weight is not in the bundled set or in the request's document fonts, or a user font is not in the workspace | `E_FONT_UNKNOWN` (SDD-000 §8) with `details.available` |
| `font_invalid` | -10 | a font file fails the validation of SDD-007 §7.7, or a font a request uses does not parse | `E_VALIDATION` with `details.reason` |
| `font_needed` | -11 | a request names a user font the module does not hold; `detail` is its digest | the host supplies the file and repeats the request (§4.2.1); never a failure of its own |
| `text_failed` | -12 | the bytes a host supplied do not hash to their digest, or the font source failed | export: `E_EXPORT_FAILED` |
| `render_trap` | none (a failed call) | the instance trapped (`unreachable`, a stack overflow, an out-of-bounds access the module's checks missed) | export: `E_EXPORT_FAILED`; preview: `error`, instance replaced |
| `device_lost` | none | the WebGPU device was lost (§4.7 rule 5) | preview only |
| `unsupported_format` | none (not an error) | the browser's `VideoDecoder.isConfigSupported` says no for the proxy's codec (§5.3.1), or the chunk index is invalid or missing (§6.3, §6.4) | preview only: the asset is decoded by `<video>`; it is counted in `stats.fallbackAssets`, and is a `warning` in the log of the worker, never an `error` |

### 7.2 Limits

The renderer enforces these itself, as the last defence, even though the product already refuses most of them earlier (SDD-007 §9.1: an export is at most 4096 pixels on a side, a document has at most 50 tracks, the ingest refuses sources above 8192 on a side). They are counted in the module's memory, not in the host's.

| Limit | Default | `details.limit` | Notes |
| --- | --- | --- | --- |
| Output frame side | 4096 pixels, so a frame is at most 64 MiB | `render_frame_side` | the "frame too large" case; `value` is the side asked for, `ceiling` 4096 |
| Layers in one plan | 64 | `render_layers` | 50 tracks give at most 50 layers; the margin is for a future overlay layer |
| Render input | 4 MiB | `render_input_bytes` | a caption with all 5,000 words of the document is about 60 KiB |
| One texture or bitmap | 8192 pixels on a side | `render_texture_side` | the normalized media is at most 3840 on its long edge (SDD-011 §1.5) |
| Textures in memory | 512 MiB in all | `render_texture_bytes` | `max_texture_bytes` of `vr_init`; the layers of one frame (§5.2) |
| Bitmaps in memory | 128 MiB in all | `render_bitmap_bytes` | `max_bitmap_bytes` of `vr_init`; premultiplied copies included |
| Lookup tables in memory | 32 tables and 64 MiB in all | `render_luts` | a table of 65 points on a side is 1.6 MiB; `vr_put_lut` fails and changes nothing |
| Module memory | 1 GiB | none | the declared maximum of §4.1 and the `wazero` limit; the budgets above plus the scratch of one layer and the 64 MiB output stay under it |
| Blur radius | 128 source pixels | none | SDD-004 §3.6; a larger blur is drawn as 128, not refused |

A `put` that would pass a budget fails with `limit_exceeded` and changes nothing; it does not evict. In the export the textures are those of one frame (§5.2), so only an extreme project reaches a budget (64 layers of 3840 x 2160 pixels is 2 GiB): it is refused as `E_LIMIT_EXCEEDED` with `details.limit = "render_texture_bytes"`, `value` the bytes of the textures of that frame, `ceiling` 512 MiB and the hint "use smaller clips, or fewer large layers at the same time". The browser worker uses the same defaults and, below them, its own smaller ones on a device that reports low memory (`navigator.deviceMemory`), set in `init` (§4.7).

### 7.3 What each side does

**Export (Go).** `renderhost.Error` carries `Reason` and `Detail` and becomes:

- `E_EXPORT_FAILED` with `details.reason = "render_failed"` (the existing reason, SDD-004 §8), `details.renderer_reason` the `reason` of §7.1 and `details.item` when `detail` names an item. The message is a sentence for a person ("A frame of the project could not be drawn."), the hint "Try again; if it repeats, report it with the output of stuarty doctor". No file path, no pixel and no key is in a message.
- `E_LIMIT_EXCEEDED` for `limit_exceeded`, with `details.limit`, `value` and `ceiling` as in SDD-007 §9.1.
- `E_CANCELLED` when the context was cancelled. A cancel is not a trap and never becomes `render_failed`.
- A trap discards the instance and fails the export once: a trap on the same input would repeat.

Nothing the renderer says is retried by the export except `missing_texture`, which is the normal upload dance of §4.3, and only once per frame.

**Preview (browser).**

- `missing_texture` and `device_lost` are handled by the worker protocol (§4.7).
- `limit_exceeded` on a texture: the main thread releases the textures of layers that left the window (§5.4) and posts the upload again once; if it fails again the layer is drawn from the fallback (`<video>` bitmap) and the failure is counted in `stats.renderErrors`.
- `out_of_memory`: the main thread releases every texture and bitmap, calls `init` again with `prefer: "cpu"` and the smaller budgets, and resends what the window needs. If that fails too the preview shows the `unsupported` state of SDD-012 §11.2 item 6 (paused frames only, Play disabled, a banner that says why).
- Any other reason is a bug of the product: it is logged with `console.error` and counted in `stats.renderErrors`, the canvas keeps the last good frame, and the editor shows no toast (SDD-012 §11.3: a failed frame is invisible, truth on pause shows the exact frame).

### 7.4 What the renderer does with hostile input

The module receives frames decoded from untrusted media and plans built from a document an agent wrote. Every length is checked against the block the host passed, every size against §7.2 before any allocation, and the arithmetic of §3 cannot read or write outside the output block or a stored texture (the loops are bounded by the clipped rectangle of §3.6). A number the plan could never produce (a `NaN`, a negative size, an opacity of 400) is clamped as §3.3 says or refused as `invalid_plan`; it is never a reason to read memory. Because the core module has no import (§4.1), a defect that nevertheless executes is confined to the instance's memory, which is discarded after a trap. This is the threat row that ADR-0029 adds to SDD-007, and its test is `TestSecurity_T36_RenderHostSurvivesHostileInput` below.

### 7.5 Acceptance of this section

- `TestRenderReasons` (Go and Rust): for each reason of §7.1 with a status, an input that causes it returns that status and, through `renderhost`, that `Reason`: an unknown field, version 2, kind `"video"`, a layer with no texture, 65 layers, a 4097-pixel output, a texture of 8193 pixels, a 5 MiB input, an `out_len` that is one byte short, a texture whose pixels are one byte short.
- `TestSecurity_T36_RenderHostSurvivesHostileInput` (Go): 10,000 random and mutated render inputs and textures (a fuzz target, `FuzzRenderInput`) never trap, never allocate beyond the budgets and never return a status outside §4.5; a plan with `NaN` as text is `invalid_plan`.
- `TestExportMapsRendererErrors` (Go): each `renderhost.Error` becomes the code and `details` of §7.3 (a table test), and a cancel becomes `E_CANCELLED`.
- A Playwright test: forcing `out_of_memory` in the worker (a tiny `max_texture_bytes`) leaves the preview on the paused-frame state with the banner, and forcing `device_lost` switches to the CPU path and keeps drawing.
