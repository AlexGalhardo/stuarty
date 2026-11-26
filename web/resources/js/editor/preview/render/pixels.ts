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

export interface PlaneLayoutLike {
	offset: number;
	stride: number;
}

export interface ColorSpaceLike {
	matrix?: string | null;
	fullRange?: boolean | null;
}

function clampByte(v: number): number {
	if (v <= 0) return 0;
	if (v >= 255) return 255;
	return Math.round(v);
}

// Converts native VideoFrame plane bytes (I420, I420A, I422, I444, NV12, or
// 32-bit RGB/BGR) to straight sRGB RGBA for `crop`. Used when the browser
// ignores `format: "RGBA"` in VideoFrame.copyTo (WebKitGTK writes native YUV
// planes instead of converting).
export function decodeVideoPlanes(
	bytes: Uint8Array,
	layouts: readonly PlaneLayoutLike[],
	format: string | null,
	colorSpace: ColorSpaceLike | null | undefined,
	crop: Whole,
): Uint8Array {
	const out = new Uint8Array(crop.w * crop.h * 4);
	if (layouts.length === 0) return out;
	const p0 = layouts[0];
	if (
		format === "RGBA" ||
		format === "RGBX" ||
		format === "BGRA" ||
		format === "BGRX"
	) {
		const swapRB = format === "BGRA" || format === "BGRX";
		const opaque = format === "RGBX" || format === "BGRX";
		for (let row = 0; row < crop.h; row++) {
			const srcRow = p0.offset + (crop.y + row) * p0.stride + crop.x * 4;
			const dstRow = row * crop.w * 4;
			for (let col = 0; col < crop.w; col++) {
				const s = srcRow + col * 4;
				const d = dstRow + col * 4;
				out[d] = bytes[swapRB ? s + 2 : s] ?? 0;
				out[d + 1] = bytes[s + 1] ?? 0;
				out[d + 2] = bytes[swapRB ? s : s + 2] ?? 0;
				out[d + 3] = opaque ? 255 : (bytes[s + 3] ?? 255);
			}
		}
		return out;
	}

	const bt601 =
		colorSpace?.matrix === "bt470bg" || colorSpace?.matrix === "smpte170m";
	const fullRange = colorSpace?.fullRange === true;
	const yScale = fullRange ? 1 : 255 / 219;
	const cScale = fullRange ? 1 : 255 / 224;
	const yOff = fullRange ? 0 : 16;
	const kr = bt601 ? 1.402 : 1.5748;
	const kgU = bt601 ? 0.344136 : 0.187324;
	const kgV = bt601 ? 0.714136 : 0.468124;
	const kb = bt601 ? 1.772 : 1.8556;

	const nv12 = format === "NV12" || layouts.length === 2;
	const p1 = layouts[1];
	const p2 = layouts[2];
	const p3 = format === "I420A" ? layouts[3] : undefined;
	const shiftX = format === "I444" ? 0 : 1;
	const shiftY = format === "I422" || format === "I444" ? 0 : 1;

	for (let row = 0; row < crop.h; row++) {
		const py = crop.y + row;
		const cy = py >> shiftY;
		const yRow = p0.offset + py * p0.stride;
		const cRow1 = p1 ? p1.offset + cy * p1.stride : 0;
		const cRow2 = p2 ? p2.offset + cy * p2.stride : 0;
		const aRow = p3 ? p3.offset + py * p3.stride : 0;
		const dstRow = row * crop.w * 4;
		for (let col = 0; col < crop.w; col++) {
			const px = crop.x + col;
			const y = ((bytes[yRow + px] ?? 0) - yOff) * yScale;
			let uRaw = 128;
			let vRaw = 128;
			if (nv12 && p1) {
				const uvIdx = cRow1 + ((px >> 1) << 1);
				uRaw = bytes[uvIdx] ?? 128;
				vRaw = bytes[uvIdx + 1] ?? 128;
			} else if (p1 && p2) {
				const cx = px >> shiftX;
				uRaw = bytes[cRow1 + cx] ?? 128;
				vRaw = bytes[cRow2 + cx] ?? 128;
			}
			const u = (uRaw - 128) * cScale;
			const v = (vRaw - 128) * cScale;
			const d = dstRow + col * 4;
			out[d] = clampByte(y + kr * v);
			out[d + 1] = clampByte(y - kgU * u - kgV * v);
			out[d + 2] = clampByte(y + kb * u);
			out[d + 3] = p3 ? (bytes[aRow + px] ?? 255) : 255;
		}
	}
	return out;
}
