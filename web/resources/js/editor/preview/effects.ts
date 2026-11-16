import type { PlanEffect, PlanLayer, PlanTransition } from "../api/types";

// The pixel effects of SDD-004 section 3.13 and the transitions of section 3.2
// that change pixels (whip blur, dip, wipe), as the passes the preview draws.
// This file turns what the plan says into the numbers of each pass with the
// integer formulas of the SDD; the filter in effectFilter.ts runs them on the
// GPU in floating point, so a pixel can differ from the export by a rounding
// step (and the glow is approximated, see SDD-012 section 11.7).

// What a pass does; the numbers are the `kind` uniform of the shader.
export const FX = {
	motion_blur: 0,
	glow: 1,
	vignette: 2,
	grain: 3,
	rgb_split: 4,
	whip: 5,
	dip: 6,
	wipe: 7,
} as const;

export interface EffectPass {
	// One of FX.
	kind: number;
	// Up to seven numbers, per kind (the table in effectFilter.ts).
	p: [number, number, number, number, number, number, number];
	// Output pixels the layer grows by around its edge.
	padding: number;
}

// `T[d] = round(1024 sin(d degrees))` of SDD-004 section 3.13.
export const SIN_TABLE: readonly number[] = [
	0, 18, 36, 54, 71, 89, 107, 125, 143, 160, 178, 195, 213, 230, 248, 265, 282,
	299, 316, 333, 350, 367, 384, 400, 416, 433, 449, 465, 481, 496, 512, 527,
	543, 558, 573, 587, 602, 616, 630, 644, 658, 672, 685, 698, 711, 724, 737,
	749, 761, 773, 784, 796, 807, 818, 828, 839, 849, 859, 868, 878, 887, 896,
	904, 912, 920, 928, 935, 943, 949, 956, 962, 968, 974, 979, 984, 989, 994,
	998, 1002, 1005, 1008, 1011, 1014, 1016, 1018, 1020, 1022, 1023, 1023, 1024,
	1024,
];

const roundHalfAway = (v: number): number =>
	Math.sign(v) * Math.round(Math.abs(v));
const roundHalfUp = (v: number): number => Math.floor(v + 0.5);

// The cosine and the sine of an angle in degrees, times 1024, from the table.
export function angleCS(angle: number): { c: number; s: number } {
	let a = roundHalfAway(angle);
	if (a > 180) a -= 360;
	if (a < -180) a += 360;
	const q = Math.abs(a) % 180;
	const u = q <= 90 ? q : 180 - q;
	const s = SIN_TABLE[u] ?? 0;
	const c = SIN_TABLE[90 - u] ?? 0;
	return { s: a >= 0 ? s : -s, c: Math.abs(a) <= 90 ? c : -c };
}

// The direction of an angle of the layer on the screen: the table's vector
// turned by the rotation of the layer, because the effect works on the layer's
// own pixels and the preview filters the layer after the rotation.
export function screenDirection(
	angle: number,
	rotation: number,
): [number, number] {
	const { c, s } = angleCS(angle);
	const r = (rotation * Math.PI) / 180;
	const x = c / 1024;
	const y = s / 1024;
	return [x * Math.cos(r) - y * Math.sin(r), x * Math.sin(r) + y * Math.cos(r)];
}

// The length of the whip blur in frame pixels (SDD-004 section 3.2).
export function whipLength(p: number, size: number): number {
	return ((((65536 - p) * size) >> 4) + 32768) >> 16;
}

// The share of the layer kept by the dip, 0 to 255 (SDD-004 section 3.2).
export function dipOpacity(p: number): number {
	return (p * 255 + 32768) >> 16;
}

// The amplitude `amq` of the grain (SDD-004 section 3.13).
export function grainAmplitude(amount: number): number {
	const m = roundHalfAway(amount * 1000);
	return Math.floor((m * 64 + 500) / 1000);
}

const MAX_TAPS = 64;

export function hexColour(color: string | undefined): [number, number, number] {
	const hex = /^#([0-9a-fA-F]{6})$/.exec(color ?? "");
	if (hex === null) return [0, 0, 0];
	const n = Number.parseInt(hex[1] as string, 16);
	return [((n >> 16) & 255) / 255, ((n >> 8) & 255) / 255, (n & 255) / 255];
}

const DIRECTION_CODE = { left: 0, right: 1, up: 2, down: 3 } as const;

export interface PassContext {
	// Output pixels per project pixel.
	scale: number;
	// The layer's rotation in degrees at the frame.
	rotation: number;
	// The frame inside the plan entry of the layer (the grain changes every
	// frame; the pattern restarts at the start of a plan window, which noise
	// does not show).
	itemFrame: number;
	// The project frame size, for the whip blur.
	frameWidth: number;
	frameHeight: number;
}

function effectPass(effect: PlanEffect, ctx: PassContext): EffectPass | null {
	const amount = effect.amount ?? 0;
	switch (effect.kind) {
		case "motion_blur": {
			const n = roundHalfUp(amount * 60 * ctx.scale);
			if (n < 2) return null;
			const h = Math.min(Math.floor(n / 2), MAX_TAPS);
			const [dx, dy] = screenDirection(effect.angle ?? 0, ctx.rotation);
			return {
				kind: FX.motion_blur,
				p: [h, dx, dy, 0, 0, 0, 0],
				padding: h + 1,
			};
		}
		case "glow": {
			const sigma = (effect.radius ?? 12) * ctx.scale;
			return {
				kind: FX.glow,
				p: [
					Math.round(amount * 255) / 255,
					sigma,
					Math.round((effect.threshold ?? 0.6) * 255) / 255,
					0,
					0,
					0,
					0,
				],
				padding: Math.ceil(sigma * 2.5) + 1,
			};
		}
		case "vignette":
			return {
				kind: FX.vignette,
				p: [
					Math.round(amount * 255) / 255,
					effect.softness ?? 0.5,
					0,
					0,
					0,
					0,
					0,
				],
				padding: 0,
			};
		case "grain":
			return {
				kind: FX.grain,
				p: [
					grainAmplitude(amount),
					Math.max(1, roundHalfUp((effect.size ?? 1) * ctx.scale)),
					effect.seed ?? 0,
					ctx.itemFrame,
					0,
					0,
					0,
				],
				padding: 0,
			};
		case "rgb_split": {
			const d = roundHalfUp(amount * 24 * ctx.scale);
			const [dx, dy] = screenDirection(effect.angle ?? 0, ctx.rotation);
			return {
				kind: FX.rgb_split,
				p: [dx * d, dy * d, 0, 0, 0, 0, 0],
				padding: d + 1,
			};
		}
		default:
			return null;
	}
}

function transitionPass(
	transition: PlanTransition,
	ctx: PassContext,
): { pass: EffectPass | null; after: boolean } {
	const { kind, p } = transition;
	if (kind === "dip") {
		const [r, g, b] = hexColour(transition.color);
		return {
			pass: {
				kind: FX.dip,
				p: [dipOpacity(p) / 255, r, g, b, 0, 0, 0],
				padding: 0,
			},
			after: false,
		};
	}
	const direction = transition.direction ?? "left";
	if (kind === "wipe") {
		return {
			pass: {
				kind: FX.wipe,
				p: [p / 65536, DIRECTION_CODE[direction], 0, 0, 0, 0, 0],
				padding: 0,
			},
			after: true,
		};
	}
	const vertical = direction === "up" || direction === "down";
	const len = whipLength(p, vertical ? ctx.frameHeight : ctx.frameWidth);
	if (len < 2) return { pass: null, after: false };
	const n = Math.min(Math.floor(roundHalfUp(len * ctx.scale) / 2), MAX_TAPS);
	if (n < 1) return { pass: null, after: false };
	const [dx, dy] = screenDirection(vertical ? 90 : 0, ctx.rotation);
	return {
		pass: { kind: FX.whip, p: [n, dx, dy, 0, 0, 0, 0], padding: n + 1 },
		after: false,
	};
}

// The passes of a layer at a frame: `before` runs ahead of the mask (the pixel
// effects in list order, then the whip blur or the dip), `after` is the wipe,
// which multiplies the mask coverage (SDD-004 section 3.13, "Where they sit").
export function effectPasses(
	layer: PlanLayer,
	frame: number,
	ctx: Omit<PassContext, "itemFrame">,
): { before: EffectPass[]; after: EffectPass[] } {
	const full = { ...ctx, itemFrame: Math.max(frame - layer.first, 0) };
	const before: EffectPass[] = [];
	const after: EffectPass[] = [];
	for (const effect of layer.effects ?? []) {
		const pass = effectPass(effect, full);
		if (pass !== null) before.push(pass);
	}
	const transition = layer.transition?.[frame - layer.first] ?? null;
	if (transition !== null) {
		const made = transitionPass(transition, full);
		if (made.pass !== null) (made.after ? after : before).push(made.pass);
	}
	return { before, after };
}
