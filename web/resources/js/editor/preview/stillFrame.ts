// The widest poster the preview asks for. Ingest never upscales, and it never
// exceeds this width (ScaleMinWidth).
export const STILL_POSTER_WIDTH = 640;

// The crop of a still, in the pixels of the bitmap the preview actually holds.
// The plan's crop is in source pixels (SDD-004 section 3.4). The poster is that
// source scaled uniformly to a width of at most 640, and never upscaled
// (ScaleMinWidth). When the caller does not know the source size, the bitmap
// is the source, which is true for a still at most 640 wide.

export interface StillRect {
	x: number;
	y: number;
	width: number;
	height: number;
}

export function stillFrameRect(
	crop: readonly [number, number, number, number],
	bitmapWidth: number,
	bitmapHeight: number,
	sourceWidth = bitmapWidth,
	sourceHeight = bitmapHeight,
): StillRect {
	const [x, y, w, h] = crop;
	const sx = sourceWidth > 0 ? bitmapWidth / sourceWidth : 1;
	const sy = sourceHeight > 0 ? bitmapHeight / sourceHeight : 1;
	return { x: x * sx, y: y * sy, width: w * sx, height: h * sy };
}
