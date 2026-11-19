import type { Rect } from "./protocol";

// A rectangle of a frame whole pixels wide, and the same widened to even
// coordinates. VideoFrame.copyTo converts YUV 4:2:0 to RGBA in blocks of 2 x 2,
// and a rect that starts on an odd pixel comes back shifted: the preview drew a
// crop two pixels off the export's. The worker copies the even rectangle that
// holds the crop and cuts the crop out of it.
export interface Whole {
	x: number;
	y: number;
	w: number;
	h: number;
}

export function whole(rect: Rect): Whole {
	const x = Math.max(0, Math.round(rect.x));
	const y = Math.max(0, Math.round(rect.y));
	return {
		x,
		y,
		w: Math.max(1, Math.round(rect.w)),
		h: Math.max(1, Math.round(rect.h)),
	};
}

export function evenAround(area: Whole, width: number, height: number): Whole {
	const x = area.x - (area.x % 2);
	const y = area.y - (area.y % 2);
	const right = Math.min(width, area.x + area.w + ((area.x + area.w) % 2));
	const bottom = Math.min(height, area.y + area.h + ((area.y + area.h) % 2));
	return { x, y, w: right - x, h: bottom - y };
}

// The part `inner` of RGBA pixels `outer` is the area of, copied row by row.
export function cutRows(
	rgba: Uint8Array,
	outer: Whole,
	inner: Whole,
): Uint8Array {
	const out = new Uint8Array(inner.w * inner.h * 4);
	const stride = outer.w * 4;
	for (let row = 0; row < inner.h; row++) {
		const from = (inner.y - outer.y + row) * stride + (inner.x - outer.x) * 4;
		out.set(rgba.subarray(from, from + inner.w * 4), row * inner.w * 4);
	}
	return out;
}
