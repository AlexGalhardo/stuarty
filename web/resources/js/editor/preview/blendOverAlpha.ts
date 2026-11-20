import { BlendModeFilter, ExtensionType, extensions } from "pixi.js";

import { OVER_ALPHA_MODES, type OverAlphaMode, overAlphaName } from "./blend";

// The blend modes of SDD-004 section 3.8 over a canvas that is not opaque (VV-268). The fixed-function
// blend of PixiJS for `multiply` is `src x dst + dst x (1 - sa)`, which is black where nothing is drawn
// yet, and its `overlay` shader has the same hole; the export draws the general form of SDD-013 section
// 3.11.2 there, which is the source itself over a clear pixel and adds the `c x (1 - da)` term over a
// partly covered one. So when the project background is clear, these four modes are drawn by a filter
// that reads the backdrop and evaluates that form in floating point, on the premultiplied values the
// filter receives. Where the backdrop is opaque (`da == 255`) the filter takes the closed forms of SDD-004
// section 3.8, as the export does; `overlay` uses its filter on an opaque canvas too, since the one of
// `pixi.js/advanced-blend-modes` reads the premultiplied source as the straight one. The GPU works in floats, so a pixel can differ from the export by a
// rounding step, inside the tolerance of ADR-0003.
//
// PixiJS draws the result of a filter over the backdrop with its `normal` blend (`ONE, ONE_MINUS_SRC_ALPHA`),
// which adds `back x (1 - out.a)`. So the shader returns what that sum turns into the general form: the
// colour `c x (1 - da) + a x da x B` with the alpha `a` (the blend adds the `d x (1 - a)` and the
// `da x (1 - a)` terms), nothing where `a == 0`, and the closed form with alpha 1 over an opaque backdrop.

// The backdrop counts as opaque from 254.5 / 255.
const OPAQUE = "0.998";

interface Forms {
	// B(s, b) of the table of SDD-013 section 3.11.2, on straight values 0 to 1.
	general: string;
	// The destination after the closed form of SDD-004 section 3.8 (premultiplied source c, alpha a).
	opaque: string;
}

const FORMS: Record<OverAlphaMode, { gl: Forms; gpu: Forms }> = {
	multiply: {
		gl: {
			general: "s * b",
			opaque: "d * (c + (1.0 - a))",
		},
		gpu: {
			general: "s * b",
			opaque: "d * (c + (1.0 - a))",
		},
	},
	screen: {
		gl: {
			general: "s + b - s * b",
			opaque: "c + d - c * d",
		},
		gpu: {
			general: "s + b - s * b",
			opaque: "c + d - c * d",
		},
	},
	overlay: {
		gl: {
			general:
				"mix(1.0 - 2.0 * (1.0 - s) * (1.0 - b), 2.0 * s * b, step(b, vec3(0.5)))",
			opaque:
				"mix(c + d * (1.0 - a) + (2.0 * d - 1.0) * (a - c), d * (2.0 * c + (1.0 - a)), step(d, vec3(0.5)))",
		},
		gpu: {
			general:
				"select(1.0 - 2.0 * (1.0 - s) * (1.0 - b), 2.0 * s * b, b <= vec3<f32>(0.5))",
			opaque:
				"select(c + d * (1.0 - a) + (2.0 * d - 1.0) * (a - c), d * (2.0 * c + (1.0 - a)), d <= vec3<f32>(0.5))",
		},
	},
	add: {
		gl: {
			general: "min(s + b, vec3(1.0))",
			opaque: "min(c + d, vec3(1.0))",
		},
		gpu: {
			general: "min(s + b, vec3<f32>(1.0))",
			opaque: "min(c + d, vec3<f32>(1.0))",
		},
	},
};

export function glShader(mode: OverAlphaMode): {
	functions: string;
	main: string;
} {
	const forms = FORMS[mode].gl;
	return {
		functions: `
			vec4 blendOverAlpha(vec4 front, vec4 back)
			{
				float a = front.a;
				float da = back.a;
				vec3 c = front.rgb;
				vec3 d = back.rgb;
				if (a <= 0.0) return vec4(0.0);
				if (da >= ${OPAQUE}) return vec4(min(${forms.opaque}, vec3(1.0)), 1.0);
				vec3 s = min(c / a, vec3(1.0));
				vec3 b = da > 0.0 ? min(d / da, vec3(1.0)) : vec3(0.0);
				vec3 blended = ${forms.general};
				return vec4(c * (1.0 - da) + a * da * blended, a);
			}
		`,
		main: `
			finalColor = blendOverAlpha(front, back) * uBlend;
		`,
	};
}

export function gpuShader(mode: OverAlphaMode): {
	functions: string;
	main: string;
} {
	const forms = FORMS[mode].gpu;
	return {
		functions: `
			fn blendOverAlpha(front: vec4<f32>, back: vec4<f32>) -> vec4<f32>
			{
				let a = front.a;
				let da = back.a;
				let c = front.rgb;
				let d = back.rgb;
				if (a <= 0.0) { return vec4<f32>(0.0); }
				if (da >= ${OPAQUE}) { return vec4<f32>(min(${forms.opaque}, vec3<f32>(1.0)), 1.0); }
				let s = min(c / a, vec3<f32>(1.0));
				var b = vec3<f32>(0.0);
				if (da > 0.0) { b = min(d / da, vec3<f32>(1.0)); }
				let blended = ${forms.general};
				return vec4<f32>(c * (1.0 - da) + a * da * blended, a);
			}
		`,
		main: `
			out = blendOverAlpha(front, back) * blendUniforms.uBlend;
		`,
	};
}

function filterClass(mode: OverAlphaMode): new () => BlendModeFilter {
	return class OverAlphaBlend extends BlendModeFilter {
		static extension = {
			name: overAlphaName(mode),
			type: ExtensionType.BlendMode,
		};

		constructor() {
			super({ gl: glShader(mode), gpu: gpuShader(mode) });
		}
	};
}

let registered = false;

// Makes the four modes known to PixiJS; calling it again does nothing.
export function registerOverAlphaBlends(): void {
	if (registered) return;
	registered = true;
	for (const mode of OVER_ALPHA_MODES) extensions.add(filterClass(mode));
}
