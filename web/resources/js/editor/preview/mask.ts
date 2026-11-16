import type { PlanMask } from "../api/types";

// The masks of SDD-004 section 3.9 in the preview. `maskCoverage` is the integer
// reference of the spec (the Go compositor, byte for byte) for one pixel of the
// layer's own frame. The preview evaluates it once per texel of a coverage
// texture (`coverageTexels`) and the GPU multiplies the layer by that texture
// through the PixiJS mask filter in the item's own frame, so the mask follows
// position, scale and rotation. The texture is capped in size and sampled
// bilinearly, so an edge can differ from the export by a fraction of a pixel
// (SDD-012 section 11.3).

// The longest side, in texels, of a coverage texture.
export const MAX_COVERAGE_SIDE = 640;

function roundHalfAway(v: number): number {
	return Math.sign(v) * Math.round(Math.abs(v));
}

function isqrt(n: number): number {
	let x = Math.floor(Math.sqrt(n));
	while (x * x > n) x--;
	while ((x + 1) * (x + 1) <= n) x++;
	return x;
}

// The geometry of a mask in 24.8 fixed point (SDD-004 section 3.9 steps 2 and 4)
// for a layer rectangle of `w x h` output pixels.
export interface MaskShape {
	ellipse: boolean;
	invert: boolean;
	left: number;
	top: number;
	a: number;
	b: number;
	feather: number;
}

export function maskShape(
	mask: PlanMask,
	w: number,
	h: number,
	outScale: number,
): MaskShape {
	const wq = roundHalfAway(w * 256);
	const hq = roundHalfAway(h * 256);
	const xm = roundHalfAway(mask.x * 1000);
	const ym = roundHalfAway(mask.y * 1000);
	const wm = roundHalfAway(mask.width * 1000);
	const hm = roundHalfAway(mask.height * 1000);
	return {
		ellipse: mask.shape === "ellipse",
		invert: mask.invert === true,
		left: Math.floor((xm * wq + 500) / 1000),
		top: Math.floor((ym * hq + 500) / 1000),
		a: Math.max(1, Math.floor((wm * wq + 1000) / 2000)),
		b: Math.max(1, Math.floor((hm * hq + 1000) / 2000)),
		feather: roundHalfAway((mask.feather ?? 0) * outScale * 256),
	};
}

// The coverage, 0 to 255, of the pixel centre `(lx, ly)` (24.8, from the
// top-left corner of the layer rectangle).
export function maskCoverage(s: MaskShape, lx: number, ly: number): number {
	let d: number;
	if (s.ellipse) {
		const qx = Math.min(
			Math.floor((Math.abs(lx - (s.left + s.a)) * 65536) / s.a),
			2 ** 22,
		);
		const qy = Math.min(
			Math.floor((Math.abs(ly - (s.top + s.b)) * 65536) / s.b),
			2 ** 22,
		);
		const r = isqrt(qx * qx + qy * qy);
		d = Math.floor(((65536 - r) * Math.min(s.a, s.b)) / 65536);
	} else {
		d = Math.min(
			lx - s.left,
			s.left + 2 * s.a - lx,
			ly - s.top,
			s.top + 2 * s.b - ly,
		);
	}
	let cov: number;
	if (s.feather === 0) {
		cov = d >= 0 ? 255 : 0;
	} else {
		const t = Math.min(Math.max(2 * d + s.feather, 0), 2 * s.feather);
		cov = Math.floor((255 * t + s.feather) / (2 * s.feather));
	}
	return s.invert ? 255 - cov : cov;
}

// The size of the coverage texture of a layer rectangle: the rectangle in
// output pixels, shrunk to fit MAX_COVERAGE_SIDE.
export function coverageSize(
	w: number,
	h: number,
): { width: number; height: number } {
	const f = Math.min(1, MAX_COVERAGE_SIDE / Math.max(w, h, 1));
	return {
		width: Math.max(1, Math.round(w * f)),
		height: Math.max(1, Math.round(h * f)),
	};
}

// RGBA texels (premultiplied, every channel the coverage) of a `width x height`
// grid laid over the layer rectangle `w x h` (output pixels). When the grid is
// the rectangle itself the texels are exactly the pixels of the export.
export function coverageTexels(
	mask: PlanMask,
	w: number,
	h: number,
	outScale: number,
	width: number,
	height: number,
): Uint8Array {
	const s = maskShape(mask, w, h, outScale);
	const out = new Uint8Array(width * height * 4);
	const sx = (w * 256) / width;
	const sy = (h * 256) / height;
	for (let y = 0; y < height; y++) {
		const ly = Math.round((y + 0.5) * sy);
		for (let x = 0; x < width; x++) {
			const cov = maskCoverage(s, Math.round((x + 0.5) * sx), ly);
			const i = (y * width + x) * 4;
			out[i] = cov;
			out[i + 1] = cov;
			out[i + 2] = cov;
			out[i + 3] = cov;
		}
	}
	return out;
}
