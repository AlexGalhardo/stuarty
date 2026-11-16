// The GPU path of SDD-013 section 3.8: one textured quad per layer, drawn bottom first with
// premultiplied source-over. The quad corners and the opacity come from the CPU path's own
// placement, so only the sampling and the blend are decided here.

struct Frame {
	size: vec2<f32>,
}

struct Layer {
	// The width and height of the layer's texture, in texels.
	texels: vec2<f32>,
	// The opacity, 0..255 on the CPU path, as a fraction.
	opacity: f32,
	pad: f32,
}

@group(0) @binding(0) var<uniform> frame: Frame;
@group(0) @binding(1) var<uniform> layer: Layer;
@group(0) @binding(2) var source: texture_2d<f32>;

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
// half integers), edges clamped: the sampler of SDD-013 section 3.6 in floating point.
@fragment
fn fs_main(in: Varying) -> @location(0) vec4<f32> {
	let f = in.uv - vec2<f32>(0.5, 0.5);
	let base = floor(f);
	let w = f - base;
	let i = vec2<i32>(base);
	let top = mix(texel(i), texel(i + vec2<i32>(1, 0)), w.x);
	let bottom = mix(texel(i + vec2<i32>(0, 1)), texel(i + vec2<i32>(1, 1)), w.x);
	return mix(top, bottom, w.y) * layer.opacity;
}
