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

@group(0) @binding(0) var<uniform> frame: Frame;
@group(0) @binding(1) var<uniform> layer: Layer;
@group(0) @binding(2) var source: texture_2d<f32>;
@group(0) @binding(3) var backdrop: texture_2d<f32>;

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

@fragment
fn fs_main(in: Varying) -> @location(0) vec4<f32> {
	return sampled(in.uv);
}

// The mix of SDD-004 section 3.8 on the opaque backdrop, in floating point: `s` is the premultiplied
// source with the opacity applied and `d` the destination colour. The destination alpha stays 1.
@fragment
fn fs_blend(in: Varying) -> @location(0) vec4<f32> {
	let s = sampled(in.uv);
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
