// Fit and size of the preview canvas (SDD-012 section 11.4).
export const PADDING_PX = 16;
export const BUFFER_LONG_EDGE_CAP = 1080;
const FRAME_WIDTHS = [270, 360, 540, 720, 1080] as const;

export function fitScale(
	availableWidth: number,
	availableHeight: number,
	width: number,
	height: number,
): number {
	if (width <= 0 || height <= 0) return 0;
	const w = Math.max(availableWidth - 2 * PADDING_PX, 0);
	const h = Math.max(availableHeight - 2 * PADDING_PX, 0);
	return Math.min(w / width, h / height);
}

// The scale of the canvas for a size choice (SDD-012 section 11.4): Fit is the
// scale that fills the region; 50% and 100% are that share of the pixel buffer,
// whose long edge is capped at 1080, so a project larger than the cap
// is shown at its buffer size and not at its own.
export function sizedScale(
	size: "fit" | "50" | "100",
	fit: number,
	width: number,
	height: number,
): number {
	if (size === "fit") return fit;
	const long = Math.max(width, height);
	if (long <= 0) return 0;
	const full = Math.min(1, BUFFER_LONG_EDGE_CAP / long);
	return size === "50" ? full / 2 : full;
}

// The backing store: the box in device pixels, capped at 1080 on the long edge.
export function bufferSize(
	width: number,
	height: number,
	scale: number,
	devicePixelRatio: number,
): { width: number; height: number } {
	let w = width * scale * devicePixelRatio;
	let h = height * scale * devicePixelRatio;
	const long = Math.max(w, h);
	if (long > BUFFER_LONG_EDGE_CAP) {
		const shrink = BUFFER_LONG_EDGE_CAP / long;
		w *= shrink;
		h *= shrink;
	}
	return {
		width: Math.max(Math.round(w), 1),
		height: Math.max(Math.round(h), 1),
	};
}

// The next of 270, 360, 540, 720, 1080, so the core's frame cache is hit.
export function truthWidth(devicePixels: number): number {
	for (const width of FRAME_WIDTHS) if (devicePixels <= width) return width;
	return 1080;
}
