import { Filter, GlProgram, GpuProgram, UniformGroup } from "pixi.js";

import type { PlanChromaKey } from "../api/types";

// The chroma key of SDD-004 section 3.7 in the preview: the same settings, the
// same BT.601 chroma distance, the same hard radius and linear ramp, the same
// spill on the dominant channel. `keyPixel` is the integer reference (the Go
// compositor, byte for byte); the filter below computes the same on the GPU in
// floats, so a pixel can differ from the export by a rounding step.

// A similarity or a smoothness of 1 stands for 128 chroma units of 1/256.
const CHROMA_SCALE = 128 * 256;

export interface ChromaSettings {
	cb: number;
	cr: number;
	t0: number;
	t1: number;
	spill: number; // 16.16
	dominant: number; // 0 red, 1 green, 2 blue, -1 none
}

function clamp01(v: number): number {
	return Math.min(Math.max(v, 0), 1);
}

// Half away from zero, as Go math.Round.
function roundHalfAway(v: number): number {
	return Math.sign(v) * Math.round(Math.abs(v));
}

export function chromaOf(
	r: number,
	g: number,
	b: number,
): [cb: number, cr: number] {
	return [-43 * r - 85 * g + 128 * b, 128 * r - 107 * g - 21 * b];
}

export function parseKeyColor(color: string): [number, number, number] {
	const hex = /^#([0-9a-fA-F]{6})(?:[0-9a-fA-F]{2})?$/.exec(color);
	if (hex === null) return [0, 255, 0];
	const n = Number.parseInt(hex[1], 16);
	return [(n >> 16) & 255, (n >> 8) & 255, n & 255];
}

export function chromaSettings(key: PlanChromaKey): ChromaSettings {
	const c = parseKeyColor(key.color);
	const [cb, cr] = chromaOf(c[0], c[1], c[2]);
	let dominant = -1;
	for (let ch = 0; ch < 3; ch++) {
		if (c[ch] > c[(ch + 1) % 3] && c[ch] > c[(ch + 2) % 3]) dominant = ch;
	}
	return {
		cb,
		cr,
		t0: roundHalfAway(clamp01(key.similarity) * CHROMA_SCALE),
		t1: roundHalfAway(clamp01(key.smoothness) * CHROMA_SCALE),
		spill: roundHalfAway(clamp01(key.spill) * 65536),
		dominant,
	};
}

function isqrt(n: number): number {
	let x = Math.floor(Math.sqrt(n));
	while (x * x > n) x--;
	while ((x + 1) * (x + 1) <= n) x++;
	return x;
}

// The opacity kept, 0 to 255, by a straight colour.
export function keepOf(
	s: ChromaSettings,
	r: number,
	g: number,
	b: number,
): number {
	const [cb, cr] = chromaOf(r, g, b);
	const d = isqrt((cb - s.cb) ** 2 + (cr - s.cr) ** 2);
	if (d <= s.t0) return 0;
	if (d >= s.t0 + s.t1) return 255;
	return Math.floor(((d - s.t0) * 255 + Math.floor(s.t1 / 2)) / s.t1);
}

function div255(v: number): number {
	return Math.floor((v + 127) / 255);
}

// One pixel through the key: premultiplied RGBA in, premultiplied RGBA out.
export function keyPixel(
	s: ChromaSettings,
	px: readonly [number, number, number, number],
): [number, number, number, number] {
	const a = px[3];
	if (a === 0) return [px[0], px[1], px[2], px[3]];
	const c: number[] = [px[0], px[1], px[2]];
	if (a !== 255) {
		for (let i = 0; i < 3; i++) {
			c[i] = Math.min(Math.floor((c[i] * 255 + Math.floor(a / 2)) / a), 255);
		}
	}
	const f = keepOf(s, c[0], c[1], c[2]);
	if (f === 0) return [0, 0, 0, 0];
	if (s.dominant >= 0 && s.spill !== 0) {
		const limit = Math.floor(
			(c[(s.dominant + 1) % 3] + c[(s.dominant + 2) % 3]) / 2,
		);
		const excess = c[s.dominant] - limit;
		if (excess > 0) {
			c[s.dominant] -= Math.floor((excess * s.spill + 32768) / 65536);
		}
	}
	const na = div255(a * f);
	return [div255(c[0] * na), div255(c[1] * na), div255(c[2] * na), na];
}

const FRAGMENT = `
in vec2 vTextureCoord;
out vec4 finalColor;
uniform sampler2D uTexture;
uniform vec4 uKey;
uniform vec4 uSpill;

void main() {
	vec4 c = texture(uTexture, vTextureCoord);
	if (c.a <= 0.0) {
		finalColor = c;
		return;
	}
	vec3 rgb = min(c.rgb / c.a, vec3(1.0));
	vec3 v = rgb * 255.0;
	vec2 chroma = vec2(
		-43.0 * v.r - 85.0 * v.g + 128.0 * v.b,
		128.0 * v.r - 107.0 * v.g - 21.0 * v.b
	);
	float d = length(chroma - uKey.xy);
	float f = 1.0;
	if (d <= uKey.z) {
		f = 0.0;
	} else if (d < uKey.z + uKey.w) {
		f = (d - uKey.z) / uKey.w;
	}
	if (f <= 0.0) {
		finalColor = vec4(0.0);
		return;
	}
	// uSpill.yzw is one-hot on the dominant channel (all zero: none). No integer modulus and no
	// indexing of a vector by a variable: a WebGL context that compiles a fragment shader as
	// GLSL ES 1.00 refuses both, and the layer then draws nothing (VV-093).
	// The two other channels are summed in 0..255 as the integer reference does, (c1 + c2) / 2: a
	// product by 0 or 1 is exact, so no step is lost to rounding a sum of all three less one.
	vec3 dom = uSpill.yzw;
	if (dom.x + dom.y + dom.z > 0.0 && uSpill.x > 0.0) {
		float kept = dot(rgb, dom);
		float others = dot(rgb * 255.0, vec3(1.0) - dom);
		float limit = floor(others / 2.0) / 255.0;
		float excess = kept - limit;
		if (excess > 0.0) {
			rgb -= dom * excess * uSpill.x;
		}
	}
	float a = c.a * f;
	finalColor = vec4(rgb * a, a);
}
`;

const VERTEX = `
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

struct KeyUniforms {
	uKey: vec4<f32>,
	uSpill: vec4<f32>,
};

@group(0) @binding(0) var<uniform> gfu: GlobalFilterUniforms;
@group(0) @binding(1) var uTexture: texture_2d<f32>;
@group(0) @binding(2) var uSampler: sampler;
@group(1) @binding(0) var<uniform> keyUniforms: KeyUniforms;

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

@fragment
fn mainFragment(@location(0) uv: vec2<f32>) -> @location(0) vec4<f32> {
	let c = textureSample(uTexture, uSampler, uv);
	if (c.a <= 0.0) {
		return c;
	}
	var rgb = min(c.rgb / c.a, vec3<f32>(1.0));
	let v = rgb * 255.0;
	let chroma = vec2<f32>(
		-43.0 * v.r - 85.0 * v.g + 128.0 * v.b,
		128.0 * v.r - 107.0 * v.g - 21.0 * v.b
	);
	let d = length(chroma - keyUniforms.uKey.xy);
	var f = 1.0;
	if (d <= keyUniforms.uKey.z) {
		f = 0.0;
	} else if (d < keyUniforms.uKey.z + keyUniforms.uKey.w) {
		f = (d - keyUniforms.uKey.z) / keyUniforms.uKey.w;
	}
	if (f <= 0.0) {
		return vec4<f32>(0.0);
	}
	let dom = keyUniforms.uSpill.yzw;
	if (dom.x + dom.y + dom.z > 0.0 && keyUniforms.uSpill.x > 0.0) {
		let kept = dot(rgb, dom);
		let others = dot(rgb * 255.0, vec3<f32>(1.0) - dom);
		let limit = floor(others / 2.0) / 255.0;
		let excess = kept - limit;
		if (excess > 0.0) {
			rgb = rgb - dom * excess * keyUniforms.uSpill.x;
		}
	}
	let a = c.a * f;
	return vec4<f32>(rgb * a, a);
}
`;

export class ChromaKeyFilter extends Filter {
	constructor() {
		const keyUniforms = new UniformGroup({
			uKey: { value: new Float32Array(4), type: "vec4<f32>" },
			uSpill: { value: new Float32Array(4), type: "vec4<f32>" },
		});
		super({
			glProgram: GlProgram.from({
				vertex: VERTEX,
				fragment: FRAGMENT,
				name: "chroma-key-filter",
				// The chroma distances reach 32640, past what a mediump float must hold (2^14): ask for
				// highp, which Pixi lowers to mediump only where the GPU has no highp.
				preferredFragmentPrecision: "highp",
			}),
			gpuProgram: GpuProgram.from({
				vertex: { source: WGSL, entryPoint: "mainVertex" },
				fragment: { source: WGSL, entryPoint: "mainFragment" },
			}),
			resources: { keyUniforms },
		});
	}

	set(key: PlanChromaKey): void {
		const s = chromaSettings(key);
		const u = this.resources.keyUniforms.uniforms;
		u.uKey = new Float32Array([s.cb, s.cr, s.t0, s.t1]);
		u.uSpill = new Float32Array([
			s.spill / 65536,
			s.dominant === 0 ? 1 : 0,
			s.dominant === 1 ? 1 : 0,
			s.dominant === 2 ? 1 : 0,
		]);
	}
}

// The fragment shader of the filter, for the test that keeps out of it what a GLSL ES 1.00 compile refuses.
export const CHROMA_FRAGMENT_SOURCE = FRAGMENT;
