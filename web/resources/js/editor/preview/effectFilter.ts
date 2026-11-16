import { Filter, GlProgram, GpuProgram, UniformGroup } from "pixi.js";

import type { EffectPass } from "./effects";

// One filter that draws any pass of effects.ts: the pixel effects of SDD-004
// section 3.13 and the whip blur, the dip and the wipe of section 3.2. The
// `kind` picks the branch of the shader, so a layer with three effects has
// three instances of this one program. The arithmetic follows the formulas of
// the SDD in floating point (the hash and the grain amplitude in whole
// numbers); the export computes the same in whole numbers, so a pixel can
// differ by a rounding step, and the glow samples a disc of 32 taps instead of
// the two Gaussian passes (SDD-012 section 11.7).
//
// The numbers of a pass, by kind (`p0` is `uP0.y`, `p1` is `uP0.z`, `p2` is
// `uP0.w`, `p3` to `p6` are `uP1`):
//   0 motion blur and 5 whip blur   p0 half taps, p1 p2 direction
//   1 glow                          p0 amount, p1 sigma, p2 threshold
//   2 vignette                      p0 amount, p1 softness
//   3 grain                         p0 amplitude, p1 cell, p2 seed, p3 frame
//   4 rgb split                     p0 p1 offset
//   6 dip                           p0 share of the layer, p1 p2 p3 colour
//   7 wipe                          p0 progress, p1 direction (left right up down)

// The integer hash needs GLSL ES 3.00 (`uint`, `textureLod`), which PixiJS only
// asks for when the source says so.
const COMMON_GLSL = `#version 300 es
precision highp float;
precision highp int;
in vec2 vTextureCoord;
out vec4 finalColor;
uniform sampler2D uTexture;
uniform vec4 uInputSize;
uniform vec4 uOutputFrame;
uniform vec4 uP0;
uniform vec4 uP1;
uniform vec4 uGeom;
uniform vec4 uRot;
`;

const FRAGMENT = `${COMMON_GLSL}
vec4 srcAt(vec2 px) {
	if (px.x < 0.0 || px.y < 0.0 || px.x >= uOutputFrame.z || px.y >= uOutputFrame.w) {
		return vec4(0.0);
	}
	return textureLod(uTexture, px * uInputSize.zw, 0.0);
}

vec2 localPos(vec2 px) {
	vec2 d = px - uGeom.xy;
	return vec2(d.x * uRot.x + d.y * uRot.y, -d.x * uRot.y + d.y * uRot.x) + uGeom.zw;
}

uint mixu(uint v) {
	v ^= v >> 16u;
	v *= 0x7feb352du;
	v ^= v >> 15u;
	v *= 0x846ca68bu;
	v ^= v >> 16u;
	return v;
}

uint hash3(uint a, uint b, uint c) {
	return mixu(a + mixu(b + mixu(c + 0x9e3779b9u)));
}

void main() {
	vec2 px = vTextureCoord * uInputSize.xy;
	vec4 c = srcAt(px);
	vec4 o = c;
	int kind = int(uP0.x + 0.5);
	if (kind == 0 || kind == 5) {
		int h = int(uP0.y + 0.5);
		vec4 sum = vec4(0.0);
		for (int k = -64; k <= 64; k++) {
			if (k < -h || k > h) continue;
			sum += srcAt(px + uP0.zw * float(k));
		}
		o = sum / float(2 * h + 1);
	} else if (kind == 1) {
		float sigma = max(uP0.z, 0.01);
		float thr = uP0.w;
		vec4 acc = vec4(0.0);
		float wsum = 0.0;
		for (int i = 0; i < 32; i++) {
			float fi = float(i) + 0.5;
			float rr = sqrt(fi / 32.0) * 2.5 * sigma;
			float ang = fi * 2.399963;
			float w = exp(-(rr * rr) / (2.0 * sigma * sigma));
			vec4 s = srcAt(px + vec2(cos(ang), sin(ang)) * rr);
			float y = dot(s.rgb, vec3(54.0, 183.0, 19.0)) / 256.0;
			float k = (thr >= 1.0 || y <= thr) ? 0.0 : min(1.0, (y - thr) / (1.0 - thr));
			acc += s * k * w;
			wsum += w;
		}
		o = min(c + acc / wsum * uP0.y, 1.0);
	} else if (kind == 2) {
		vec2 lp = localPos(px);
		float nx = min(abs(lp.x - uGeom.z) / uGeom.z, 64.0);
		float ny = min(abs(lp.y - uGeom.w) / uGeom.w, 64.0);
		float r = sqrt(nx * nx + ny * ny);
		float r1 = 92682.0 / 65536.0;
		float r0 = r1 - r1 * uP0.z;
		float f = r >= r1 ? 1.0 : (r <= r0 ? 0.0 : (r - r0) / (r1 - r0));
		float m = 1.0 - f * f * uP0.y;
		o = vec4(c.rgb * m, c.a);
	} else if (kind == 3) {
		vec2 lp = localPos(px);
		float sz = max(uP0.z, 1.0);
		uint cx = uint(max(floor(lp.x / sz), 0.0));
		uint cy = uint(max(floor(lp.y / sz), 0.0));
		uint seed = uint(uP0.w + 0.5);
		uint fr = uint(uP1.x + 0.5);
		int v = int(hash3(cx + (seed << 16u), cy, fr) >> 24u);
		int amq = int(uP0.y + 0.5);
		int delta = ((v - 128) * amq + 64) >> 7;
		int a = int(c.a * 255.0 + 0.5);
		int dl = (delta < 0 ? -1 : (delta > 0 ? 1 : 0)) * ((abs(delta) * a + 127) / 255);
		vec3 ci = clamp(floor(c.rgb * 255.0 + 0.5) + float(dl), 0.0, float(a));
		o = vec4(ci / 255.0, c.a);
	} else if (kind == 4) {
		vec4 lo = srcAt(px - uP0.yz);
		vec4 hi = srcAt(px + uP0.yz);
		o = vec4(lo.r, c.g, hi.b, max(max(lo.a, c.a), hi.a));
	} else if (kind == 6) {
		vec3 k = vec3(uP0.z, uP0.w, uP1.x);
		o = vec4(c.rgb * uP0.y + k * c.a * (1.0 - uP0.y), c.a);
	} else if (kind == 7) {
		vec2 lp = localPos(px);
		vec2 size = uGeom.zw * 2.0;
		int dir = int(uP0.z + 0.5);
		float cov = 0.0;
		if (dir == 0) cov = lp.x < uP0.y * size.x ? 1.0 : 0.0;
		else if (dir == 1) cov = lp.x >= size.x - uP0.y * size.x ? 1.0 : 0.0;
		else if (dir == 2) cov = lp.y < uP0.y * size.y ? 1.0 : 0.0;
		else cov = lp.y >= size.y - uP0.y * size.y ? 1.0 : 0.0;
		o = c * cov;
	}
	finalColor = o;
}
`;

const VERTEX = `#version 300 es
in vec2 aPosition;
out vec2 vTextureCoord;
uniform vec4 uInputSize;
uniform vec4 uOutputFrame;
uniform vec4 uOutputTexture;

void main(void) {
	vec2 position = aPosition * uOutputFrame.zw + uOutputFrame.xy;
	position.x = position.x * (2.0 / uOutputTexture.x) - 1.0;
	position.y = position.y * (2.0 * uOutputTexture.z / uOutputTexture.y) - uOutputTexture.z;
	gl_Position = vec4(position, 0.0, 1.0);
	vTextureCoord = aPosition * (uOutputFrame.zw * uInputSize.zw);
}
`;

const WGSL = `
struct GlobalFilterUniforms {
	uInputSize: vec4<f32>,
	uInputPixel: vec4<f32>,
	uInputClamp: vec4<f32>,
	uOutputFrame: vec4<f32>,
	uGlobalFrame: vec4<f32>,
	uOutputTexture: vec4<f32>,
};

struct FxUniforms {
	uP0: vec4<f32>,
	uP1: vec4<f32>,
	uGeom: vec4<f32>,
	uRot: vec4<f32>,
};

@group(0) @binding(0) var<uniform> gfu: GlobalFilterUniforms;
@group(0) @binding(1) var uTexture: texture_2d<f32>;
@group(0) @binding(2) var uSampler: sampler;
@group(1) @binding(0) var<uniform> fx: FxUniforms;

struct VSOutput {
	@builtin(position) position: vec4<f32>,
	@location(0) uv: vec2<f32>,
};

fn filterVertexPosition(aPosition: vec2<f32>) -> vec4<f32> {
	var position = aPosition * gfu.uOutputFrame.zw + gfu.uOutputFrame.xy;
	position.x = position.x * (2.0 / gfu.uOutputTexture.x) - 1.0;
	position.y = position.y * (2.0 * gfu.uOutputTexture.z / gfu.uOutputTexture.y) - gfu.uOutputTexture.z;
	return vec4(position, 0.0, 1.0);
}

fn filterTextureCoord(aPosition: vec2<f32>) -> vec2<f32> {
	return aPosition * (gfu.uOutputFrame.zw * gfu.uInputSize.zw);
}

@vertex
fn mainVertex(@location(0) aPosition: vec2<f32>) -> VSOutput {
	return VSOutput(filterVertexPosition(aPosition), filterTextureCoord(aPosition));
}

fn srcAt(px: vec2<f32>) -> vec4<f32> {
	if (px.x < 0.0 || px.y < 0.0 || px.x >= gfu.uOutputFrame.z || px.y >= gfu.uOutputFrame.w) {
		return vec4<f32>(0.0);
	}
	return textureSampleLevel(uTexture, uSampler, px * gfu.uInputSize.zw, 0.0);
}

fn localPos(px: vec2<f32>) -> vec2<f32> {
	let d = px - fx.uGeom.xy;
	return vec2<f32>(d.x * fx.uRot.x + d.y * fx.uRot.y, -d.x * fx.uRot.y + d.y * fx.uRot.x) + fx.uGeom.zw;
}

fn mixu(vin: u32) -> u32 {
	var v = vin;
	v = v ^ (v >> 16u);
	v = v * 0x7feb352du;
	v = v ^ (v >> 15u);
	v = v * 0x846ca68bu;
	v = v ^ (v >> 16u);
	return v;
}

fn hash3(a: u32, b: u32, c: u32) -> u32 {
	return mixu(a + mixu(b + mixu(c + 0x9e3779b9u)));
}

@fragment
fn mainFragment(@location(0) uv: vec2<f32>) -> @location(0) vec4<f32> {
	let px = uv * gfu.uInputSize.xy;
	let c = srcAt(px);
	var o = c;
	let kind = i32(fx.uP0.x + 0.5);
	if (kind == 0 || kind == 5) {
		let h = i32(fx.uP0.y + 0.5);
		var sum = vec4<f32>(0.0);
		for (var k = -64; k <= 64; k = k + 1) {
			if (k < -h || k > h) {
				continue;
			}
			sum = sum + srcAt(px + fx.uP0.zw * f32(k));
		}
		o = sum / f32(2 * h + 1);
	} else if (kind == 1) {
		let sigma = max(fx.uP0.z, 0.01);
		let thr = fx.uP0.w;
		var acc = vec4<f32>(0.0);
		var wsum = 0.0;
		for (var i = 0; i < 32; i = i + 1) {
			let fi = f32(i) + 0.5;
			let rr = sqrt(fi / 32.0) * 2.5 * sigma;
			let ang = fi * 2.399963;
			let w = exp(-(rr * rr) / (2.0 * sigma * sigma));
			let s = srcAt(px + vec2<f32>(cos(ang), sin(ang)) * rr);
			let y = dot(s.rgb, vec3<f32>(54.0, 183.0, 19.0)) / 256.0;
			var k = 0.0;
			if (thr < 1.0 && y > thr) {
				k = min(1.0, (y - thr) / (1.0 - thr));
			}
			acc = acc + s * k * w;
			wsum = wsum + w;
		}
		o = min(c + acc / wsum * fx.uP0.y, vec4<f32>(1.0));
	} else if (kind == 2) {
		let lp = localPos(px);
		let nx = min(abs(lp.x - fx.uGeom.z) / fx.uGeom.z, 64.0);
		let ny = min(abs(lp.y - fx.uGeom.w) / fx.uGeom.w, 64.0);
		let r = sqrt(nx * nx + ny * ny);
		let r1 = 92682.0 / 65536.0;
		let r0 = r1 - r1 * fx.uP0.z;
		var f = 0.0;
		if (r >= r1) {
			f = 1.0;
		} else if (r > r0) {
			f = (r - r0) / (r1 - r0);
		}
		let m = 1.0 - f * f * fx.uP0.y;
		o = vec4<f32>(c.rgb * m, c.a);
	} else if (kind == 3) {
		let lp = localPos(px);
		let sz = max(fx.uP0.z, 1.0);
		let cx = u32(max(floor(lp.x / sz), 0.0));
		let cy = u32(max(floor(lp.y / sz), 0.0));
		let seed = u32(fx.uP0.w + 0.5);
		let fr = u32(fx.uP1.x + 0.5);
		let v = i32(hash3(cx + (seed << 16u), cy, fr) >> 24u);
		let amq = i32(fx.uP0.y + 0.5);
		let delta = ((v - 128) * amq + 64) >> 7u;
		let a = i32(c.a * 255.0 + 0.5);
		var sgn = 0;
		if (delta < 0) {
			sgn = -1;
		} else if (delta > 0) {
			sgn = 1;
		}
		let dl = sgn * ((abs(delta) * a + 127) / 255);
		let ci = clamp(floor(c.rgb * 255.0 + 0.5) + f32(dl), vec3<f32>(0.0), vec3<f32>(f32(a)));
		o = vec4<f32>(ci / 255.0, c.a);
	} else if (kind == 4) {
		let lo = srcAt(px - fx.uP0.yz);
		let hi = srcAt(px + fx.uP0.yz);
		o = vec4<f32>(lo.r, c.g, hi.b, max(max(lo.a, c.a), hi.a));
	} else if (kind == 6) {
		let k = vec3<f32>(fx.uP0.z, fx.uP0.w, fx.uP1.x);
		o = vec4<f32>(c.rgb * fx.uP0.y + k * c.a * (1.0 - fx.uP0.y), c.a);
	} else if (kind == 7) {
		let lp = localPos(px);
		let size = fx.uGeom.zw * 2.0;
		let dir = i32(fx.uP0.z + 0.5);
		var cov = 0.0;
		if (dir == 0) {
			if (lp.x < fx.uP0.y * size.x) { cov = 1.0; }
		} else if (dir == 1) {
			if (lp.x >= size.x - fx.uP0.y * size.x) { cov = 1.0; }
		} else if (dir == 2) {
			if (lp.y < fx.uP0.y * size.y) { cov = 1.0; }
		} else {
			if (lp.y >= size.y - fx.uP0.y * size.y) { cov = 1.0; }
		}
		o = c * cov;
	}
	return o;
}
`;

// Where the layer sits inside the filter texture, in output pixels: the centre
// from the top-left corner of the texture, half of the size and the rotation.
export interface PassGeometry {
	cx: number;
	cy: number;
	halfWidth: number;
	halfHeight: number;
	rotation: number;
}

export class EffectFilter extends Filter {
	constructor() {
		const fx = new UniformGroup({
			uP0: { value: new Float32Array(4), type: "vec4<f32>" },
			uP1: { value: new Float32Array(4), type: "vec4<f32>" },
			uGeom: { value: new Float32Array(4), type: "vec4<f32>" },
			uRot: { value: new Float32Array(4), type: "vec4<f32>" },
		});
		super({
			glProgram: GlProgram.from({
				vertex: VERTEX,
				fragment: FRAGMENT,
				name: "effect-filter",
			}),
			gpuProgram: GpuProgram.from({
				vertex: { source: WGSL, entryPoint: "mainVertex" },
				fragment: { source: WGSL, entryPoint: "mainFragment" },
			}),
			resources: { fx },
			// The area is set by the executor, which knows where the texture sits.
			clipToViewport: false,
		});
	}

	set(pass: EffectPass, geometry: PassGeometry): void {
		const u = this.resources.fx.uniforms;
		const [p0, p1, p2, p3, p4, p5, p6] = pass.p;
		u.uP0 = new Float32Array([pass.kind, p0, p1, p2]);
		u.uP1 = new Float32Array([p3, p4, p5, p6]);
		const r = (geometry.rotation * Math.PI) / 180;
		u.uGeom = new Float32Array([
			geometry.cx,
			geometry.cy,
			geometry.halfWidth,
			geometry.halfHeight,
		]);
		u.uRot = new Float32Array([Math.cos(r), Math.sin(r), 0, 0]);
	}
}
