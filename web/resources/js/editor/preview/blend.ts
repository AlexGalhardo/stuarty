import type { BLEND_MODES } from "pixi.js";

import type { PlanBlend } from "../api/types";

// The blend modes of SDD-004 section 3.8 in the preview. PixiJS draws
// `multiply`, `screen` and `add` with fixed-function GPU blending on
// premultiplied colours, which is the same formula as the export; `overlay`
// comes from `pixi.js/advanced-blend-modes` (a shader that reads the
// destination). The GPU works in floats, so a pixel can differ from the
// export by a rounding step; `blendChannel` is the integer reference of the
// spec (the Go compositor, byte for byte) that the tests hold the formulas to.

export function pixiBlendMode(blend: PlanBlend | undefined): BLEND_MODES {
	return blend ?? "normal";
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
