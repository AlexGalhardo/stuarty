// The GPU path of SDD-013 section 3.8: one textured quad per layer, drawn bottom first with
// premultiplied source-over. The quad corners and the opacity come from the CPU path's own
// placement, so only the sampling and the blend are decided here. A layer with a blend mode
// (SDD-004 section 3.8) reads the frame drawn so far from `backdrop`, a copy the host takes before
// the layer, and writes the mixed pixel itself with the fixed blend state off.

struct Frame {
	size: vec2<f32>,
}

struct Layer {
	// The width and height of the layer's texture, in texels.
	texels: vec2<f32>,
	// The opacity, 0..255 on the CPU path, as a fraction.
	opacity: f32,
	// The blend mode: 1 multiply, 2 screen, 3 overlay, 4 add, 5 soft light, 6 hard light, 7 darken,
	// 8 lighten, 9 colour dodge, 10 colour burn, 11 difference (0 is the fixed-function path).
	mode: u32,
}

// The mask of SDD-004 section 3.9 in pixels of the layer's own rectangle (all zero when the layer
// has none). `map_x` is (ax, bx, ox, enabled) and `map_y` is (ay, by, oy, wipe direction): the
// position of the centre of the pixel (x, y) in the rectangle is (ax x + bx y + ox, ay x + by y + oy).
// `enabled` is 1 for a shape and 2 for a wipe alone (no shape: the coverage of the shape is 1). The wipe
// direction is 0 for none, 1 left, 2 right, 3 up and 4 down (SDD-004 section 3.2). `shape` is (left,
// top, half width, half height) and `edge` is (feather, shape, invert, the edge of the wipe) with shape 0 a
// rectangle, 1 an ellipse and 2 a rounded rectangle (SDD-013 section 3.11.3): the
// wipe covers `u < edge` (left), `u >= edge` (right), `v < edge` (up) or `v >= edge` (down).
struct Mask {
	map_x: vec4<f32>,
	map_y: vec4<f32>,
	shape: vec4<f32>,
	edge: vec4<f32>,
	// (corner radius in pixels, 0, 0, 0): the radius of a rounded rectangle, which `edge.y` says is shape 2.
	extra: vec4<f32>,
}

@group(0) @binding(0) var<uniform> frame: Frame;
@group(0) @binding(1) var<uniform> layer: Layer;
@group(0) @binding(2) var source: texture_2d<f32>;
@group(0) @binding(3) var backdrop: texture_2d<f32>;
@group(0) @binding(4) var<uniform> mask: Mask;

struct Varying {
	@builtin(position) position: vec4<f32>,
	@location(0) uv: vec2<f32>,
}

@vertex
fn vs_main(@location(0) pixel: vec2<f32>, @location(1) uv: vec2<f32>) -> Varying {
	var out: Varying;
	let ndc = vec2<f32>(pixel.x / frame.size.x * 2.0 - 1.0, 1.0 - pixel.y / frame.size.y * 2.0);
	out.position = vec4<f32>(ndc, 0.0, 1.0);
	out.uv = uv;
	return out;
}

fn texel(at: vec2<i32>) -> vec4<f32> {
	let last = vec2<i32>(layer.texels) - vec2<i32>(1, 1);
	return textureLoad(source, clamp(at, vec2<i32>(0, 0), last), 0);
}

// Bilinear interpolation of the four nearest texels of the point `uv` (in texels, texel centres at
// half integers), edges clamped, times the opacity: the sampler of SDD-013 section 3.6 in floating
// point.
fn sampled(uv: vec2<f32>) -> vec4<f32> {
	let f = uv - vec2<f32>(0.5, 0.5);
	let base = floor(f);
	let w = f - base;
	let i = vec2<i32>(base);
	let top = mix(texel(i), texel(i + vec2<i32>(1, 0)), w.x);
	let bottom = mix(texel(i + vec2<i32>(0, 1)), texel(i + vec2<i32>(1, 1)), w.x);
	return mix(top, bottom, w.y) * layer.opacity;
}

// The coverage of the pixel under `at` (a pixel centre) as a fraction: the coverage of SDD-004
// section 3.9 in floating point, 1 for a layer with no mask. It multiplies the sampled pixel before
// the blend, as on the CPU path.
fn coverage(at: vec2<f32>) -> f32 {
	if mask.map_x.w == 0.0 {
		return 1.0;
	}
	let p = at - vec2<f32>(0.5, 0.5);
	let l = vec2<f32>(
		mask.map_x.x * p.x + mask.map_x.y * p.y + mask.map_x.z,
		mask.map_y.x * p.x + mask.map_y.y * p.y + mask.map_y.z,
	);
	var cov = 1.0;
	if mask.map_x.w == 1.0 {
		let half = mask.shape.zw;
		let corner = mask.shape.xy;
		var d: f32;
		if mask.edge.y == 2.0 {
			let rq = min(mask.extra.x, min(half.x, half.y));
			let q = abs(l - corner - half) - (half - vec2<f32>(rq, rq));
			if q.x > 0.0 || q.y > 0.0 {
				d = rq - length(max(q, vec2<f32>(0.0, 0.0)));
			} else {
				d = rq - max(q.x, q.y);
			}
		} else if mask.edge.y != 0.0 {
			let q = abs(l - corner - half) / half;
			d = (1.0 - length(q)) * min(half.x, half.y);
		} else {
			let far = corner + 2.0 * half;
			d = min(min(l.x - corner.x, far.x - l.x), min(l.y - corner.y, far.y - l.y));
		}
		if mask.edge.x == 0.0 {
			cov = select(0.0, 1.0, d >= 0.0);
		} else {
			cov = clamp(d / mask.edge.x + 0.5, 0.0, 1.0);
		}
		cov = select(cov, 1.0 - cov, mask.edge.z != 0.0);
	}
	let wipe = u32(mask.map_y.w);
	if wipe != 0u {
		var inside: bool;
		if wipe == 1u {
			inside = l.x < mask.edge.w;
		} else if wipe == 2u {
			inside = l.x >= mask.edge.w;
		} else if wipe == 3u {
			inside = l.y < mask.edge.w;
		} else {
			inside = l.y >= mask.edge.w;
		}
		cov = cov * select(0.0, 1.0, inside);
	}
	return cov;
}

@fragment
fn fs_main(in: Varying) -> @location(0) vec4<f32> {
	return sampled(in.uv) * coverage(in.position.xy);
}

fn hard_light(s: vec3<f32>, b: vec3<f32>) -> vec3<f32> {
	let t = 2.0 * s - vec3<f32>(1.0);
	return select(b + t - b * t, b * 2.0 * s, s <= vec3<f32>(0.5));
}

fn soft_light(s: vec3<f32>, b: vec3<f32>) -> vec3<f32> {
	let d = select(sqrt(b), ((16.0 * b - vec3<f32>(12.0)) * b + vec3<f32>(4.0)) * b, b <= vec3<f32>(0.25));
	let low = b - (vec3<f32>(1.0) - 2.0 * s) * b * (vec3<f32>(1.0) - b);
	let high = b + (2.0 * s - vec3<f32>(1.0)) * (d - b);
	return select(high, low, s <= vec3<f32>(0.5));
}

// The separable blend function B(s, b) of the straight source and destination colours, the formulas
// of SDD-013 section 3.11.2 in floating point.
fn blend_fn(mode: u32, s: vec3<f32>, b: vec3<f32>) -> vec3<f32> {
	let one = vec3<f32>(1.0);
	let zero = vec3<f32>(0.0);
	if mode == 1u {
		return s * b;
	} else if mode == 2u {
		return s + b - s * b;
	} else if mode == 3u {
		return hard_light(b, s);
	} else if mode == 4u {
		return min(s + b, one);
	} else if mode == 5u {
		return soft_light(s, b);
	} else if mode == 6u {
		return hard_light(s, b);
	} else if mode == 7u {
		return min(s, b);
	} else if mode == 8u {
		return max(s, b);
	} else if mode == 9u {
		let q = min(one, b / max(one - s, vec3<f32>(1e-6)));
		return select(select(q, one, s >= one), zero, b <= zero);
	} else if mode == 10u {
		let q = one - min(one, (one - b) / max(s, vec3<f32>(1e-6)));
		return select(select(q, zero, s <= zero), one, b >= one);
	}
	return abs(s - b);
}

// The mix of SDD-004 section 3.8 on the opaque backdrop, in floating point: `s` is the premultiplied
// source with the opacity applied and `d` the destination colour. The first four modes keep the
// destination alpha on an opaque backdrop; the others, and every mode over a backdrop with alpha
// below 1, are the general form of SDD-013 section 3.11.2.
@fragment
fn fs_blend(in: Varying) -> @location(0) vec4<f32> {
	let s = sampled(in.uv) * coverage(in.position.xy);
	let back = textureLoad(backdrop, vec2<i32>(in.position.xy), 0);
	let d = back.rgb;
	let c = s.rgb;
	let ia = 1.0 - s.a;
	if s.a <= 0.0 {
		return back;
	}
	if layer.mode <= 4u && back.a >= 1.0 {
		var v: vec3<f32>;
		if layer.mode == 1u {
			v = d * (c + vec3<f32>(ia));
		} else if layer.mode == 2u {
			v = c + d - c * d;
		} else if layer.mode == 3u {
			let low = d * (2.0 * c + vec3<f32>(ia));
			let high = c + d * ia + (2.0 * d - vec3<f32>(1.0)) * (vec3<f32>(s.a) - c);
			v = select(high, low, d <= vec3<f32>(0.5));
		} else {
			v = c + d;
		}
		return vec4<f32>(clamp(v, vec3<f32>(0.0), vec3<f32>(1.0)), 1.0);
	}
	if back.a <= 0.0 {
		return s;
	}
	let sc = clamp(c / s.a, vec3<f32>(0.0), vec3<f32>(1.0));
	let bc = clamp(d / back.a, vec3<f32>(0.0), vec3<f32>(1.0));
	let t = s.a * back.a;
	let out_c = c * (1.0 - back.a) + d * ia + t * blend_fn(layer.mode, sc, bc);
	return vec4<f32>(clamp(out_c, vec3<f32>(0.0), vec3<f32>(1.0)), s.a + back.a - t);
}
