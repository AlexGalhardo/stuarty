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
   5. The colour matrix (§3.4), then the filter matrix (§3.4), then the blur (§3.5), each only when the layer has it.
   6. Draw (§3.6) with `op`.

### 3.4 Colour matrix and filter

The matrix `m` is 12 numbers: `m[4i + j]` is the coefficient of channel `j` for output `i` and `m[4i + 3]` is the offset. Each coefficient is converted once to `q[k] = round_half_away(mul(m[k], 65536))` (16.16 fixed point, `i64`). For each source pixel with alpha `a`:

- `a == 0`: the pixel is copied unchanged.
- Otherwise take the straight colour `(r, g, b)`: as it is for `a == 255`, else `min((c x 255 + a / 2) / a, 255)` with integer division.
- `nr = clamp8((q[0] x r + q[1] x g + q[2] x b + q[3] x 255 + 32768) >> 16)`, and `ng`, `nb` the same with `q[4..7]` and `q[8..11]`. The shift is arithmetic on `i64`.
- For `a != 255` premultiply again: `n = div255(n x a)`. Alpha is unchanged.

The result goes to scratch memory, never over the cached source. The filter matrix runs the same routine on the output of the colour matrix.

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

### 3.8 The GPU path

The GPU path (stage D, `wgpu`: WebGPU in the browser, Vulkan, Metal or D3D12 natively) is not bit-exact and does not try to be: float arithmetic differs across vendors and drivers. It follows the same order of operations (§3.3) and is held to the CPU path by:

- **SSIM** of at least 0.99 on the luma of the whole frame, luma being BT.709 `0.2126 R + 0.7152 G + 0.0722 B` of the final 8-bit values, computed by the SSIM routine of the SDD-004 §11 tests;
- **geometry** within 0.5 pixel: for every layer of the golden cases, each of the four corners of the drawn rectangle, as the GPU draws it, differs from the CPU path's corner by at most 0.5 output pixel. The quads are transformed on the CPU in `f64` and handed to the GPU as final positions, so this checks the pipeline and is not a new freedom.

Both are checked on every golden case, in three browsers, and on the three operating systems for the native host. A case that fails is a bug of the GPU path; the CPU path is never loosened to match it.

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
- Stage D gate: §3.8 on every golden case.
