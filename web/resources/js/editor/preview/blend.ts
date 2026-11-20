import type { BLEND_MODES } from "pixi.js";

import type { PlanBlend } from "../api/types";

// The blend modes of SDD-004 section 3.8 in the preview. Over an opaque canvas PixiJS draws `multiply`,
// `screen` and `add` with fixed-function GPU blending on premultiplied colours, which is the same formula
// as the export. `overlay` always, and the four over a canvas that is not opaque, are drawn by the filters
// of blendOverAlpha.ts, which read the destination (the `overlay` of `pixi.js/advanced-blend-modes` takes
// the premultiplied source for the straight one, so below opacity 1 it is not the export's). The GPU
// works in floats, so a pixel can differ from the export by a rounding step; `blendChannel` is the
// integer reference of the spec (the Go compositor, byte for byte) that the tests hold the formulas to.

export type OverAlphaMode = Extract<
	PlanBlend,
	"multiply" | "screen" | "overlay" | "add"
>;

export const OVER_ALPHA_MODES: readonly OverAlphaMode[] = [
	"multiply",
	"screen",
	"overlay",
	"add",
];

// The name under which blendOverAlpha.ts registers the filter that draws `mode` (VV-268).
export function overAlphaName(mode: OverAlphaMode): string {
	return `${mode}-over-alpha`;
}

// The mode drawn by a filter of blendOverAlpha.ts, or null for the blend of PixiJS.
function overAlphaMode(
	blend: PlanBlend | undefined,
	clear: boolean,
): OverAlphaMode | null {
	if (blend === "overlay") return "overlay";
	if (
		clear &&
		(blend === "multiply" || blend === "screen" || blend === "add")
	) {
		return blend;
	}
	return null;
}

// `clear` is true when the project background is not opaque: the four modes of the export are then
// drawn by the general form of SDD-013 section 3.11.2, which the fixed-function blend of PixiJS is not.
// The name is not one of the BLEND_MODES of PixiJS, which takes any registered one.
export function pixiBlendMode(
	blend: PlanBlend | undefined,
	clear = false,
): BLEND_MODES {
	const mode = overAlphaMode(blend, clear);
	if (mode !== null) return overAlphaName(mode) as BLEND_MODES;
	return blend ?? "normal";
}

// True when the layer is drawn by a filter that reads the pixels drawn so far: on WebGL that filter
// needs the back buffer, without which PixiJS skips it and draws the layer as `normal`.
export function readsBackdrop(
	blend: PlanBlend | undefined,
	clear = false,
): boolean {
	return overAlphaMode(blend, clear) !== null;
}

// div255 of SDD-004 section 3.8, exact for x from 0 to 65535.
export function div255(x: number): number {
	return (x + 128 + ((x + 128) >> 8)) >> 8;
}

// One destination channel after drawing a premultiplied source channel `c`
// (alpha `a`, opacity applied) over the opaque destination channel `d`.
export function blendChannel(
	mode: PlanBlend,
	c: number,
	a: number,
	d: number,
): number {
	if (a === 0) return d;
	const ia = 255 - a;
	let v: number;
	switch (mode) {
		case "multiply":
			v = div255(d * (c + ia));
			break;
		case "screen":
			v = c + d - div255(c * d);
			break;
		case "overlay":
			v =
				d <= 127
					? div255(d * (2 * c + ia))
					: c + div255(d * ia + (2 * d - 255) * (a - c));
			break;
		case "add":
			v = c + d;
			break;
		default:
			v = c + div255(d * ia);
	}
	return Math.min(v, 255);
}
