import {
	BufferImageSource,
	Filter,
	GlProgram,
	GpuProgram,
	UniformGroup,
} from "pixi.js";

import type { PlanGrade } from "../api/types";
import { type GradeUniforms, gradeIsIdentity, gradeUniforms } from "./grade";

// The advanced colour of SDD-004 section 3.14 in the preview, on the straight
// colour of the pixel: HSL bands, then wheels, then curves. The arithmetic is
// the one of the SDD in floating point; the export does it in whole numbers, so
// a pixel can differ by a rounding step. The curves are a 256 by 1 texture
// read by whole index, so they are the export's tables.

const GLSL = `#version 300 es
precision highp float;
precision highp int;
in vec2 vTextureCoord;
out vec4 finalColor;
uniform sampler2D uTexture;
uniform sampler2D uCurves;
uniform vec4 uFlags;
uniform vec4 uB0;
uniform vec4 uB1;
uniform vec4 uB2;
uniform vec4 uB3;
uniform vec4 uB4;
uniform vec4 uB5;
uniform vec4 uB6;
uniform vec4 uB7;
uniform vec4 uW0;
uniform vec4 uW1;
uniform vec4 uW2;

const float CENTRES[9] = float[9](0.0, 0.5, 1.0, 2.0, 3.0, 4.0, 4.5833333, 5.3333333, 6.0);

vec3 bandAt(int i) {
	vec4 b[8] = vec4[8](uB0, uB1, uB2, uB3, uB4, uB5, uB6, uB7);
	return b[i].xyz;
}

vec3 hslBands(vec3 c) {
	float mx = max(c.r, max(c.g, c.b));
	float mn = min(c.r, min(c.g, c.b));
	float d = mx - mn;
	if (d <= 0.0) {
		return c;
	}
	float sum = mx + mn;
	float sat = d / (sum <= 1.0 ? sum : 2.0 - sum);
	float h;
	if (mx == c.r) {
		h = (c.g - c.b) / d;
		if (h < 0.0) {
			h += 6.0;
		}
	} else if (mx == c.g) {
		h = (c.b - c.r) / d + 2.0;
	} else {
		h = (c.r - c.g) / d + 4.0;
	}
	int i = 0;
	for (int k = 0; k < 8; k++) {
		if (h >= CENTRES[k]) {
			i = k;
		}
	}
	float t = (h - CENTRES[i]) / (CENTRES[i + 1] - CENTRES[i]);
	float wa = (1.0 - t) * sat;
	float wb = t * sat;
	vec3 a = bandAt(i);
	vec3 b = bandAt((i + 1) % 8);
	float hs = (wa * a.x + wb * b.x) * 0.5;
	float ds = wa * a.y + wb * b.y;
	float dl = wa * a.z + wb * b.z;
	if (hs == 0.0 && ds == 0.0 && dl == 0.0) {
		return c;
	}
	float h2 = mod(h + hs, 6.0);
	float s2 = clamp(sat * (1.0 + ds), 0.0, 1.0);
	float l = sum * 0.5;
	l = dl >= 0.0 ? l + (1.0 - l) * dl : l + l * dl;
	float ch = (1.0 - abs(2.0 * l - 1.0)) * s2;
	float x = ch * (1.0 - abs(mod(h2, 2.0) - 1.0));
	float m = l - ch * 0.5;
	int sector = int(floor(h2));
	vec3 o;
	if (sector == 0) {
		o = vec3(ch, x, 0.0);
	} else if (sector == 1) {
		o = vec3(x, ch, 0.0);
	} else if (sector == 2) {
		o = vec3(0.0, ch, x);
	} else if (sector == 3) {
		o = vec3(0.0, x, ch);
	} else if (sector == 4) {
		o = vec3(x, 0.0, ch);
	} else {
		o = vec3(ch, 0.0, x);
	}
	return clamp(o + m, 0.0, 1.0);
}

vec3 wheels(vec3 c) {
	float y = (54.0 * c.r + 183.0 * c.g + 19.0 * c.b) / 256.0;
	float shadows = (1.0 - y) * (1.0 - y);
	float highlights = y * y;
	float midtones = max(1.0 - shadows - highlights, 0.0);
	vec3 add = shadows * (uW0.xyz + uW0.w) + midtones * (uW1.xyz + uW1.w) + highlights * (uW2.xyz + uW2.w);
	return clamp(c + add, 0.0, 1.0);
}

vec3 curves(vec3 c) {
	ivec3 i = ivec3(floor(clamp(c, 0.0, 1.0) * 255.0 + 0.5));
	return vec3(
		texelFetch(uCurves, ivec2(i.r, 0), 0).r,
		texelFetch(uCurves, ivec2(i.g, 0), 0).g,
		texelFetch(uCurves, ivec2(i.b, 0), 0).b
	);
}

void main() {
	vec4 c = texture(uTexture, vTextureCoord);
	if (c.a <= 0.0) {
		finalColor = c;
		return;
	}
	vec3 rgb = min(c.rgb / c.a, vec3(1.0));
	if (uFlags.x > 0.5) {
		rgb = hslBands(rgb);
	}
	if (uFlags.y > 0.5) {
		rgb = wheels(rgb);
	}
	if (uFlags.z > 0.5) {
		rgb = curves(rgb);
	}
	finalColor = vec4(rgb * c.a, c.a);
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

struct GradeUniforms {
	uFlags: vec4<f32>,
	uB0: vec4<f32>,
	uB1: vec4<f32>,
	uB2: vec4<f32>,
	uB3: vec4<f32>,
	uB4: vec4<f32>,
	uB5: vec4<f32>,
	uB6: vec4<f32>,
	uB7: vec4<f32>,
	uW0: vec4<f32>,
	uW1: vec4<f32>,
	uW2: vec4<f32>,
};

@group(0) @binding(0) var<uniform> gfu: GlobalFilterUniforms;
@group(0) @binding(1) var uTexture: texture_2d<f32>;
@group(0) @binding(2) var uSampler: sampler;
@group(1) @binding(0) var<uniform> g: GradeUniforms;
@group(1) @binding(1) var uCurves: texture_2d<f32>;

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

const CENTRES = array<f32, 9>(0.0, 0.5, 1.0, 2.0, 3.0, 4.0, 4.5833333, 5.3333333, 6.0);

fn bandAt(i: i32) -> vec3<f32> {
	var b = array<vec4<f32>, 8>(g.uB0, g.uB1, g.uB2, g.uB3, g.uB4, g.uB5, g.uB6, g.uB7);
	return b[i].xyz;
}

fn hslBands(c: vec3<f32>) -> vec3<f32> {
	let mx = max(c.r, max(c.g, c.b));
	let mn = min(c.r, min(c.g, c.b));
	let d = mx - mn;
	if (d <= 0.0) {
		return c;
	}
	let sum = mx + mn;
	var den = sum;
	if (sum > 1.0) {
		den = 2.0 - sum;
	}
	let sat = d / den;
	var h: f32;
	if (mx == c.r) {
		h = (c.g - c.b) / d;
		if (h < 0.0) {
			h = h + 6.0;
		}
	} else if (mx == c.g) {
		h = (c.b - c.r) / d + 2.0;
	} else {
		h = (c.r - c.g) / d + 4.0;
	}
	var i = 0;
	for (var k = 0; k < 8; k = k + 1) {
		if (h >= CENTRES[k]) {
			i = k;
		}
	}
	let t = (h - CENTRES[i]) / (CENTRES[i + 1] - CENTRES[i]);
	let wa = (1.0 - t) * sat;
	let wb = t * sat;
	let a = bandAt(i);
	let b = bandAt((i + 1) % 8);
	let hs = (wa * a.x + wb * b.x) * 0.5;
	let ds = wa * a.y + wb * b.y;
	let dl = wa * a.z + wb * b.z;
	if (hs == 0.0 && ds == 0.0 && dl == 0.0) {
		return c;
	}
	let h2 = (h + hs) - 6.0 * floor((h + hs) / 6.0);
	let s2 = clamp(sat * (1.0 + ds), 0.0, 1.0);
	var l = sum * 0.5;
	if (dl >= 0.0) {
		l = l + (1.0 - l) * dl;
	} else {
		l = l + l * dl;
	}
	let ch = (1.0 - abs(2.0 * l - 1.0)) * s2;
	let x = ch * (1.0 - abs((h2 - 2.0 * floor(h2 / 2.0)) - 1.0));
	let m = l - ch * 0.5;
	let sector = i32(floor(h2));
	var o: vec3<f32>;
	if (sector == 0) {
		o = vec3<f32>(ch, x, 0.0);
	} else if (sector == 1) {
		o = vec3<f32>(x, ch, 0.0);
	} else if (sector == 2) {
		o = vec3<f32>(0.0, ch, x);
	} else if (sector == 3) {
		o = vec3<f32>(0.0, x, ch);
	} else if (sector == 4) {
		o = vec3<f32>(x, 0.0, ch);
	} else {
		o = vec3<f32>(ch, 0.0, x);
	}
	return clamp(o + vec3<f32>(m), vec3<f32>(0.0), vec3<f32>(1.0));
}

fn wheels(c: vec3<f32>) -> vec3<f32> {
	let y = (54.0 * c.r + 183.0 * c.g + 19.0 * c.b) / 256.0;
	let shadows = (1.0 - y) * (1.0 - y);
	let highlights = y * y;
	let midtones = max(1.0 - shadows - highlights, 0.0);
	let add = shadows * (g.uW0.xyz + vec3<f32>(g.uW0.w))
		+ midtones * (g.uW1.xyz + vec3<f32>(g.uW1.w))
		+ highlights * (g.uW2.xyz + vec3<f32>(g.uW2.w));
	return clamp(c + add, vec3<f32>(0.0), vec3<f32>(1.0));
}

fn curves(c: vec3<f32>) -> vec3<f32> {
	let i = vec3<i32>(floor(clamp(c, vec3<f32>(0.0), vec3<f32>(1.0)) * 255.0 + 0.5));
	return vec3<f32>(
		textureLoad(uCurves, vec2<i32>(i.r, 0), 0).r,
		textureLoad(uCurves, vec2<i32>(i.g, 0), 0).g,
		textureLoad(uCurves, vec2<i32>(i.b, 0), 0).b
	);
}

@fragment
fn mainFragment(@location(0) uv: vec2<f32>) -> @location(0) vec4<f32> {
	let c = textureSample(uTexture, uSampler, uv);
	if (c.a <= 0.0) {
		return c;
	}
	var rgb = min(c.rgb / c.a, vec3<f32>(1.0));
	if (g.uFlags.x > 0.5) {
		rgb = hslBands(rgb);
	}
	if (g.uFlags.y > 0.5) {
		rgb = wheels(rgb);
	}
	if (g.uFlags.z > 0.5) {
		rgb = curves(rgb);
	}
	return vec4<f32>(rgb * c.a, c.a);
}
`;

const UNIFORM_NAMES = [
	"uB0",
	"uB1",
	"uB2",
	"uB3",
	"uB4",
	"uB5",
	"uB6",
	"uB7",
	"uW0",
	"uW1",
	"uW2",
] as const;

const IDENTITY_CURVES = (() => {
	const t = new Uint8Array(256 * 4);
	for (let v = 0; v < 256; v++) t.set([v, v, v, 255], v * 4);
	return t;
})();

// The filter of one layer; `set` reads the grade of the plan, and returns false
// when the grade changes no pixel (the caller then leaves the filter out).
export class GradeFilter extends Filter {
	private readonly curves: BufferImageSource;
	private curvesKey = "";
	// The plan object last read: it is the same object for as long as the window
	// is, so the uniforms are not rebuilt on every frame.
	private applied: PlanGrade | null = null;
	private active = false;

	constructor() {
		const grade = new UniformGroup({
			uFlags: { value: new Float32Array(4), type: "vec4<f32>" },
			...Object.fromEntries(
				UNIFORM_NAMES.map((name) => [
					name,
					{ value: new Float32Array(4), type: "vec4<f32>" },
				]),
			),
		});
		const curves = new BufferImageSource({
			resource: IDENTITY_CURVES.slice(),
			width: 256,
			height: 1,
			format: "rgba8unorm",
			alphaMode: "no-premultiply-alpha",
			scaleMode: "nearest",
		});
		super({
			glProgram: GlProgram.from({
				vertex: VERTEX,
				fragment: GLSL,
				name: "grade-filter",
			}),
			gpuProgram: GpuProgram.from({
				vertex: { source: WGSL, entryPoint: "mainVertex" },
				fragment: { source: WGSL, entryPoint: "mainFragment" },
			}),
			resources: { grade, uCurves: curves },
		});
		this.curves = curves;
	}

	set(plan: PlanGrade): boolean {
		if (plan === this.applied) return this.active;
		this.applied = plan;
		const u: GradeUniforms = gradeUniforms(plan);
		this.active = !gradeIsIdentity(u);
		if (!this.active) return false;
		const g = this.resources.grade.uniforms;
		g.uFlags = u.flags;
		u.bands.forEach((band, i) => {
			g[`uB${i}`] = band;
		});
		u.wheels.forEach((wheel, i) => {
			g[`uW${i}`] = wheel;
		});
		const texels = u.curves ?? IDENTITY_CURVES;
		const key = texels.join(",");
		if (key !== this.curvesKey) {
			this.curvesKey = key;
			this.curves.resource = texels.slice();
			this.curves.update();
		}
		return true;
	}

	override destroy(destroyProgram?: boolean): void {
		super.destroy(destroyProgram);
		this.curves.destroy();
	}
}
