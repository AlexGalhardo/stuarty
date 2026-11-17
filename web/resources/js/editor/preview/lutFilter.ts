import {
	BufferImageSource,
	Filter,
	GlProgram,
	GpuProgram,
	UniformGroup,
} from "pixi.js";

import type { LutTable } from "../api/lut";

// The lookup table of SDD-004 section 3.6 in the preview: on the straight pixel
// quantised to a byte, trilinear over the eight entries around it, then the
// intensity mix. The index and the weight of each channel are the integer ones
// of the SDD (`p = v * (n - 1)`, `i0 = p / 255`, `f = p mod 255`); the sum is
// in floating point, so a pixel can differ from the export by a rounding step.
//
// The table is a 2D atlas so that it works on WebGL2 and WebGPU alike: the
// texel (2r, g + n*b) holds the high bytes of the three channels of the entry
// (r, g, b) and the texel (2r + 1, g + n*b) the low bytes, read by whole index.
// The texture has the size of the largest table, so a table change is an
// upload, not a new texture.

export const MAX_LUT_SIZE = 65;
const ATLAS_WIDTH = 2 * MAX_LUT_SIZE;
const ATLAS_HEIGHT = MAX_LUT_SIZE * MAX_LUT_SIZE;

const GLSL = `#version 300 es
precision highp float;
precision highp int;
in vec2 vTextureCoord;
out vec4 finalColor;
uniform sampler2D uTexture;
uniform sampler2D uLut;
uniform vec4 uLutInfo;

vec3 entryAt(ivec3 i, int n) {
	int row = i.g + n * i.b;
	vec3 hi = floor(texelFetch(uLut, ivec2(2 * i.r, row), 0).rgb * 255.0 + 0.5);
	vec3 lo = floor(texelFetch(uLut, ivec2(2 * i.r + 1, row), 0).rgb * 255.0 + 0.5);
	return hi * 256.0 + lo;
}

void main() {
	vec4 c = texture(uTexture, vTextureCoord);
	if (c.a <= 0.0) {
		finalColor = c;
		return;
	}
	int n = int(uLutInfo.x);
	vec3 rgb = min(c.rgb / c.a, vec3(1.0));
	ivec3 q = ivec3(floor(rgb * 255.0 + 0.5));
	ivec3 p = q * (n - 1);
	ivec3 i0 = p / 255;
	ivec3 i1 = min(i0 + 1, ivec3(n - 1));
	vec3 f = vec3(p - i0 * 255) / 255.0;
	vec3 acc = vec3(0.0);
	for (int k = 0; k < 8; k++) {
		bool hr = (k & 1) != 0;
		bool hg = (k & 2) != 0;
		bool hb = (k & 4) != 0;
		float w = (hr ? f.r : 1.0 - f.r) * (hg ? f.g : 1.0 - f.g) * (hb ? f.b : 1.0 - f.b);
		ivec3 at = ivec3(hr ? i1.r : i0.r, hg ? i1.g : i0.g, hb ? i1.b : i0.b);
		acc += w * entryAt(at, n);
	}
	vec3 v = acc / 65535.0;
	vec3 inq = vec3(q) / 255.0;
	vec3 o = inq * (1.0 - uLutInfo.y) + v * uLutInfo.y;
	finalColor = vec4(clamp(o, 0.0, 1.0) * c.a, c.a);
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

struct LutUniforms {
	uLutInfo: vec4<f32>,
};

@group(0) @binding(0) var<uniform> gfu: GlobalFilterUniforms;
@group(0) @binding(1) var uTexture: texture_2d<f32>;
@group(0) @binding(2) var uSampler: sampler;
@group(1) @binding(0) var<uniform> l: LutUniforms;
@group(1) @binding(1) var uLut: texture_2d<f32>;

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

fn entryAt(i: vec3<i32>, n: i32) -> vec3<f32> {
	let row = i.g + n * i.b;
	let hi = floor(textureLoad(uLut, vec2<i32>(2 * i.r, row), 0).rgb * 255.0 + 0.5);
	let lo = floor(textureLoad(uLut, vec2<i32>(2 * i.r + 1, row), 0).rgb * 255.0 + 0.5);
	return hi * 256.0 + lo;
}

@fragment
fn mainFragment(@location(0) uv: vec2<f32>) -> @location(0) vec4<f32> {
	let c = textureSample(uTexture, uSampler, uv);
	if (c.a <= 0.0) {
		return c;
	}
	let n = i32(l.uLutInfo.x);
	let rgb = min(c.rgb / c.a, vec3<f32>(1.0));
	let q = vec3<i32>(floor(rgb * 255.0 + 0.5));
	let p = q * (n - 1);
	let i0 = p / vec3<i32>(255);
	let i1 = min(i0 + vec3<i32>(1), vec3<i32>(n - 1));
	let f = vec3<f32>(p - i0 * 255) / 255.0;
	var acc = vec3<f32>(0.0);
	for (var k = 0; k < 8; k = k + 1) {
		var wr = 1.0 - f.r;
		var wg = 1.0 - f.g;
		var wb = 1.0 - f.b;
		var at = i0;
		if ((k & 1) != 0) { wr = f.r; at.r = i1.r; }
		if ((k & 2) != 0) { wg = f.g; at.g = i1.g; }
		if ((k & 4) != 0) { wb = f.b; at.b = i1.b; }
		acc = acc + wr * wg * wb * entryAt(at, n);
	}
	let v = acc / 65535.0;
	let inq = vec3<f32>(q) / 255.0;
	let o = inq * (1.0 - l.uLutInfo.y) + v * l.uLutInfo.y;
	return vec4<f32>(clamp(o, vec3<f32>(0.0), vec3<f32>(1.0)) * c.a, c.a);
}
`;

// The intensity in thousandths, half away from zero as the SDD says; 0 draws
// nothing.
export function lutMix(intensity: number): number {
	if (Number.isNaN(intensity)) return 0;
	return Math.round(Math.min(Math.max(intensity * 1000, 0), 1000));
}

// The atlas texels of a table (see the top of the file); the part of the
// texture past it is left as it was.
export function lutAtlas(table: LutTable, into: Uint8Array): void {
	const n = table.size;
	for (let b = 0; b < n; b++) {
		for (let g = 0; g < n; g++) {
			const row = g + n * b;
			for (let r = 0; r < n; r++) {
				const from = 3 * (r + n * row);
				const hi = (2 * r + row * ATLAS_WIDTH) * 4;
				const lo = hi + 4;
				for (let c = 0; c < 3; c++) {
					const v = table.entries[from + c];
					into[hi + c] = v >> 8;
					into[lo + c] = v & 255;
				}
				into[hi + 3] = 255;
				into[lo + 3] = 255;
			}
		}
	}
}

// The filter of one layer; `set` reads the table and the intensity, and returns
// false when the layer draws nothing through it.
export class LutFilter extends Filter {
	private readonly atlas: BufferImageSource;
	private table: LutTable | null = null;

	constructor() {
		const lut = new UniformGroup({
			uLutInfo: { value: new Float32Array(4), type: "vec4<f32>" },
		});
		const atlas = new BufferImageSource({
			resource: new Uint8Array(ATLAS_WIDTH * ATLAS_HEIGHT * 4),
			width: ATLAS_WIDTH,
			height: ATLAS_HEIGHT,
			format: "rgba8unorm",
			alphaMode: "no-premultiply-alpha",
			scaleMode: "nearest",
		});
		super({
			glProgram: GlProgram.from({
				vertex: VERTEX,
				fragment: GLSL,
				name: "lut-filter",
			}),
			gpuProgram: GpuProgram.from({
				vertex: { source: WGSL, entryPoint: "mainVertex" },
				fragment: { source: WGSL, entryPoint: "mainFragment" },
			}),
			resources: { lut, uLut: atlas },
		});
		this.atlas = atlas;
	}

	set(table: LutTable, intensity: number): boolean {
		const mix = lutMix(intensity);
		if (mix === 0) return false;
		if (table !== this.table) {
			this.table = table;
			lutAtlas(table, this.atlas.resource as Uint8Array);
			this.atlas.update();
		}
		this.resources.lut.uniforms.uLutInfo = new Float32Array([
			table.size,
			mix / 1000,
			0,
			0,
		]);
		return true;
	}

	override destroy(destroyProgram?: boolean): void {
		super.destroy(destroyProgram);
		this.atlas.destroy();
	}
}
