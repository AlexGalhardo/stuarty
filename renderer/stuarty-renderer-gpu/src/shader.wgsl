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
	// The blend mode: 1 multiply, 2 screen, 3 overlay, 4 add (0 is the fixed-function path).
	mode: u32,
}

// The mask of SDD-004 section 3.9 in pixels of the layer's own rectangle (all zero when the layer
// has none). `map_x` is (ax, bx, ox, enabled) and `map_y` is (ay, by, oy, wipe direction): the
// position of the centre of the pixel (x, y) in the rectangle is (ax x + bx y + ox, ay x + by y + oy).
// `enabled` is 1 for a shape and 2 for a wipe alone (no shape: the coverage of the shape is 1). The wipe
// direction is 0 for none, 1 left, 2 right, 3 up and 4 down (SDD-004 section 3.2). `shape` is (left,
// top, half width, half height) and `edge` is (feather, ellipse, invert, the edge of the wipe): the
// wipe covers `u < edge` (left), `u >= edge` (right), `v < edge` (up) or `v >= edge` (down).
struct Mask {
	map_x: vec4<f32>,
	map_y: vec4<f32>,
	shape: vec4<f32>,
	edge: vec4<f32>,
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
		if mask.edge.y != 0.0 {
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

// The mix of SDD-004 section 3.8 on the opaque backdrop, in floating point: `s` is the premultiplied
// source with the opacity applied and `d` the destination colour. The destination alpha stays 1.
@fragment
fn fs_blend(in: Varying) -> @location(0) vec4<f32> {
	let s = sampled(in.uv) * coverage(in.position.xy);
	let d = textureLoad(backdrop, vec2<i32>(in.position.xy), 0).rgb;
	let c = s.rgb;
	let ia = 1.0 - s.a;
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
	if s.a <= 0.0 {
		v = d;
	}
	return vec4<f32>(clamp(v, vec3<f32>(0.0), vec3<f32>(1.0)), 1.0);
}
