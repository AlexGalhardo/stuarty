import type { PlanLayer } from "../api/types";

// The drop shadow and the outline of SDD-004 section 3.15. The integer form is
// the export's (a disc, three box blurs, premultiplied source-over), so the
// preview's bytes match it. Lengths are project pixels. `outScale` is output
// pixels per project pixel; `drawnW` and `drawnH` are the placement in output
// pixels, before this pad.

const MAX_OUTLINE_RADIUS = 256;
const MAX_BLUR_RADIUS = 128;

export interface PlacedPixels {
	pixels: Uint8Array;
	width: number;
	height: number;
	// Top-left relative to the picture's top-left, in source pixels.
	x: number;
	y: number;
}

export interface ShadowOutlineImage {
	pixels: Uint8Array;
	width: number;
	height: number;
	shadow: PlacedPixels | null;
	outline: PlacedPixels | null;
}

type RGBA = [number, number, number, number];

export function shadowOutline(
	src: Uint8Array,
	width: number,
	height: number,
	layer: Pick<PlanLayer, "shadow" | "outline">,
	outScale: number,
	drawnW: number,
	drawnH: number,
): ShadowOutlineImage {
	const unchanged = (): ShadowOutlineImage => ({
		pixels: src,
		width,
		height,
		shadow: null,
		outline: null,
	});
	const shadow = layer.shadow;
	const outline = layer.outline;
	if (
		(shadow === undefined && outline === undefined) ||
		!(drawnW > 0 && drawnH > 0) ||
		width < 1 ||
		height < 1
	) {
		return unchanged();
	}

	let oc: RGBA = [0, 0, 0, 255];
	let ox = 0;
	let oy = 0;
	if (outline !== undefined) {
		oc = parseRGBA(outline.color);
		if (oc[3] !== 0) {
			ox = Math.min(
				srcLen(outline.width, outScale, width, drawnW),
				MAX_OUTLINE_RADIUS,
			);
			oy = Math.min(
				srcLen(outline.width, outScale, height, drawnH),
				MAX_OUTLINE_RADIUS,
			);
		}
	}

	let sc: RGBA = [0, 0, 0, 255];
	let factor = 0;
	let dxs = 0;
	let dys = 0;
	let rx = 0;
	let ry = 0;
	if (shadow !== undefined) {
		sc = parseRGBA(shadow.color);
		const op = roundHalfAway(clamp01(shadow.opacity) * 255);
		factor = div255(op * sc[3]);
		if (factor !== 0) {
			dxs = srcLenSigned(shadow.dx, outScale, width, drawnW);
			dys = srcLenSigned(shadow.dy, outScale, height, drawnH);
			const sigma = (shadow.blur * outScale) / 2;
			rx = blurRadius(sigma / (drawnW / width));
			ry = blurRadius(sigma / (drawnH / height));
		}
	}
	if (factor === 0 && ox === 0 && oy === 0) return unchanged();

	const padX = ox + 3 * rx + iabs(dxs);
	const padY = oy + 3 * ry + iabs(dys);
	const outW = width + 2 * padX;
	const outH = height + 2 * padY;
	const out = new Uint8Array(outW * outH * 4);

	let dilated: Uint8Array | null = null;
	let dw = 0;
	let dh = 0;
	if (ox !== 0 || oy !== 0) {
		const grown = dilateAlpha(src, width, height, ox, oy);
		dilated = grown.pixels;
		dw = grown.width;
		dh = grown.height;
	}

	let shadowLayer: PlacedPixels | null = null;
	if (factor !== 0) {
		const sil = shadowSilhouette(src, width, height, dilated, dw, dh, ox, oy);
		let alpha = sil.pixels;
		let sw = sil.width;
		let sh = sil.height;
		let bpx = 0;
		let bpy = 0;
		if (rx !== 0 || ry !== 0) {
			const blurred = blurAlpha(alphaOf(alpha, sw, sh), sw, sh, rx, ry);
			alpha = blurred.pixels;
			bpx = Math.floor((blurred.width - sw) / 2);
			bpy = Math.floor((blurred.height - sh) / 2);
			sw = blurred.width;
			sh = blurred.height;
		}
		const painted = colourImage(alpha, sw, sh, sc, factor);
		shadowLayer = {
			pixels: painted,
			width: sw,
			height: sh,
			x: dxs - ox - bpx,
			y: dys - oy - bpy,
		};
		blit(
			out,
			outW,
			outH,
			painted,
			sw,
			sh,
			padX + shadowLayer.x,
			padY + shadowLayer.y,
		);
	}

	let outlineLayer: PlacedPixels | null = null;
	if (dilated !== null) {
		const painted = colourImage(dilated, dw, dh, oc, oc[3]);
		outlineLayer = { pixels: painted, width: dw, height: dh, x: -ox, y: -oy };
		blit(out, outW, outH, painted, dw, dh, padX - ox, padY - oy);
	}
	blit(out, outW, outH, src, width, height, padX, padY);

	return {
		pixels: out,
		width: outW,
		height: outH,
		shadow: shadowLayer,
		outline: outlineLayer,
	};
}

// round_half_up of SDD-004 section 3.15: floor(v + 0.5), so a negative half goes
// toward +∞.
function roundHalfUp(v: number): number {
	return Math.floor(v + 0.5);
}

// math.Round: half away from zero. Opacity uses it; a positive blur radius does too.
function roundHalfAway(v: number): number {
	return (v < 0 ? -1 : 1) * Math.round(Math.abs(v));
}

function clamp01(v: number): number {
	if (!(v > 0)) return 0;
	if (v > 1) return 1;
	return v;
}

function div255(x: number): number {
	const n = (x + 128) | 0;
	return (n + (n >> 8)) >> 8;
}

function iabs(v: number): number {
	return v < 0 ? -v : v;
}

function srcLen(
	length: number,
	outScale: number,
	src: number,
	drawn: number,
): number {
	const n = srcLenSigned(length, outScale, src, drawn);
	return Math.max(n, 0);
}

function srcLenSigned(
	length: number,
	outScale: number,
	src: number,
	drawn: number,
): number {
	if (!(drawn > 0)) return 0;
	return roundHalfUp((length * outScale * src) / drawn);
}

// Three boxes of width 2r+1 have variance r(r+1). r is capped so a window sum
// fits a 16-bit lane (SDD-004 section 3.6).
function blurRadius(sigma: number): number {
	if (!(sigma > 0)) return 0;
	const square = sigma * sigma;
	const inside = 4 * square + 1;
	const r = roundHalfAway((Math.sqrt(inside) - 1) / 2);
	if (r > MAX_BLUR_RADIUS) return MAX_BLUR_RADIUS;
	if (r < 0) return 0;
	return r;
}

function inDisc(dx: number, dy: number, ox: number, oy: number): boolean {
	if (ox === 0 && oy === 0) return dx === 0 && dy === 0;
	if (ox === 0) return dx === 0 && dy * dy <= oy * oy;
	if (oy === 0) return dy === 0 && dx * dx <= ox * ox;
	const dx2 = dx * dx;
	const dy2 = dy * dy;
	const ox2 = ox * ox;
	const oy2 = oy * oy;
	return dx2 * oy2 + dy2 * ox2 <= ox2 * oy2;
}

function dilateAlpha(
	src: Uint8Array,
	srcW: number,
	srcH: number,
	ox: number,
	oy: number,
): { pixels: Uint8Array; width: number; height: number } {
	const w = srcW + 2 * ox;
	const h = srcH + 2 * oy;
	const out = new Uint8Array(w * h * 4);
	for (let y = 0; y < h; y++) {
		for (let x = 0; x < w; x++) {
			const cx = x - ox;
			const cy = y - oy;
			let maxA = 0;
			for (let dy = -oy; dy <= oy; dy++) {
				for (let dx = -ox; dx <= ox; dx++) {
					if (!inDisc(dx, dy, ox, oy)) continue;
					const sx = cx + dx;
					const sy = cy + dy;
					if (sx < 0 || sy < 0 || sx >= srcW || sy >= srcH) continue;
					const a = src[(sy * srcW + sx) * 4 + 3] ?? 0;
					if (a > maxA) maxA = a;
				}
			}
			out[(y * w + x) * 4 + 3] = maxA;
		}
	}
	return { pixels: out, width: w, height: h };
}

function shadowSilhouette(
	src: Uint8Array,
	srcW: number,
	srcH: number,
	dilated: Uint8Array | null,
	dw: number,
	dh: number,
	ox: number,
	oy: number,
): { pixels: Uint8Array; width: number; height: number } {
	if (ox === 0 && oy === 0) {
		const pix = new Uint8Array(srcW * srcH * 4);
		for (let i = 3; i < src.length && i < pix.length; i += 4)
			pix[i] = src[i] ?? 0;
		return { pixels: pix, width: srcW, height: srcH };
	}
	const pix = new Uint8Array(dw * dh * 4);
	const cover = dilated ?? pix;
	for (let y = 0; y < dh; y++) {
		for (let x = 0; x < dw; x++) {
			const ao = cover[(y * dw + x) * 4 + 3] ?? 0;
			let ap = 0;
			const sx = x - ox;
			const sy = y - oy;
			if (sx >= 0 && sy >= 0 && sx < srcW && sy < srcH) {
				ap = src[(sy * srcW + sx) * 4 + 3] ?? 0;
			}
			pix[(y * dw + x) * 4 + 3] = ap + div255(ao * (255 - ap));
		}
	}
	return { pixels: pix, width: dw, height: dh };
}

function alphaOf(pixels: Uint8Array, w: number, h: number): Uint8Array {
	const alpha = new Uint8Array(w * h);
	for (let i = 0; i < w * h; i++) alpha[i] = pixels[i * 4 + 3] ?? 0;
	return alpha;
}

// The silhouette is RGB 0, and a box blur does not mix lanes, so only the alpha
// of the three horizontal passes and the three vertical passes is kept.
function blurAlpha(
	alpha: Uint8Array,
	silW: number,
	silH: number,
	rx: number,
	ry: number,
): { pixels: Uint8Array; width: number; height: number } {
	const px = 3 * rx;
	const py = 3 * ry;
	const w = silW + 2 * px;
	const h = silH + 2 * py;
	const rows = new Uint16Array(w * h);
	const line = new Uint16Array(Math.max(w, h));
	const spare = new Uint16Array(Math.max(w, h));
	for (let y = 0; y < h; y++) {
		line.fill(0, 0, w);
		const sy = y - py;
		if (sy >= 0 && sy < silH) {
			const base = sy * silW;
			for (let x = 0; x < silW; x++) line[px + x] = alpha[base + x] ?? 0;
		}
		const blurred = blur3(line, w, rx, spare);
		rows.set(blurred.subarray(0, w), y * w);
	}
	const out = new Uint8Array(w * h * 4);
	const column = new Uint16Array(h);
	for (let x = 0; x < w; x++) {
		for (let y = 0; y < h; y++) column[y] = rows[y * w + x] ?? 0;
		const blurred = blur3(column, h, ry, spare);
		for (let y = 0; y < h; y++) out[(y * w + x) * 4 + 3] = blurred[y] ?? 0;
	}
	return { pixels: out, width: w, height: h };
}

// Three box passes. `n` is the live length; the arrays may be longer.
function blur3(
	row: Uint16Array,
	n: number,
	r: number,
	spare: Uint16Array,
): Uint16Array {
	if (r === 0) return row;
	const a = row.subarray(0, n);
	const b = spare.subarray(0, n);
	blurLine(a, b, r);
	blurLine(b, a, r);
	blurLine(a, b, r);
	return b;
}

function blurLine(row: Uint16Array, out: Uint16Array, r: number): void {
	const w = row.length;
	const div = 2 * r + 1;
	const half = Math.floor(div / 2);
	const mean = (sum: number): number => Math.floor((sum + half) / div);
	let sum = 0;
	for (let x = 0; x <= r && x < w; x++) sum += row[x] ?? 0;
	if (w <= div) {
		for (let x = 0; x < w; x++) {
			out[x] = mean(sum);
			const old = x - r;
			if (old >= 0) sum -= row[old] ?? 0;
			const incoming = x + r + 1;
			if (incoming < w) sum += row[incoming] ?? 0;
		}
		return;
	}
	for (let x = 0; x < r; x++) {
		out[x] = mean(sum);
		sum += row[x + r + 1] ?? 0;
	}
	for (let x = r; x < w - r - 1; x++) {
		out[x] = mean(sum);
		sum += (row[x + r + 1] ?? 0) - (row[x - r] ?? 0);
	}
	for (let x = w - r - 1; x < w; x++) {
		out[x] = mean(sum);
		sum -= row[x - r] ?? 0;
	}
}

function colourImage(
	alpha: Uint8Array,
	w: number,
	h: number,
	color: RGBA,
	factor: number,
): Uint8Array {
	const out = new Uint8Array(w * h * 4);
	if (factor === 0) return out;
	const pixels = alpha.length === w * h ? 1 : 4;
	for (let i = 0; i < w * h; i++) {
		const a = div255((alpha[i * pixels + (pixels - 1)] ?? 0) * factor);
		if (a === 0) continue;
		const o = i * 4;
		out[o] = div255(color[0] * a);
		out[o + 1] = div255(color[1] * a);
		out[o + 2] = div255(color[2] * a);
		out[o + 3] = a;
	}
	return out;
}

function blit(
	dst: Uint8Array,
	dstW: number,
	dstH: number,
	src: Uint8Array,
	srcW: number,
	srcH: number,
	x0: number,
	y0: number,
): void {
	for (let y = 0; y < srcH; y++) {
		const dy = y0 + y;
		if (dy < 0 || dy >= dstH) continue;
		for (let x = 0; x < srcW; x++) {
			const dx = x0 + x;
			if (dx < 0 || dx >= dstW) continue;
			const si = (y * srcW + x) * 4;
			const a = src[si + 3] ?? 0;
			if (a === 0) continue;
			overPixel(
				dst,
				(dy * dstW + dx) * 4,
				src[si] ?? 0,
				src[si + 1] ?? 0,
				src[si + 2] ?? 0,
				a,
			);
		}
	}
}

function overPixel(
	d: Uint8Array,
	at: number,
	r: number,
	g: number,
	b: number,
	a: number,
): void {
	const ia = 255 - a;
	d[at] = byte(r + div255((d[at] ?? 0) * ia));
	d[at + 1] = byte(g + div255((d[at + 1] ?? 0) * ia));
	d[at + 2] = byte(b + div255((d[at + 2] ?? 0) * ia));
	d[at + 3] = byte(a + div255((d[at + 3] ?? 0) * ia));
}

function byte(v: number): number {
	return Math.min(v, 255);
}

// #RRGGBB is alpha 255. #RRGGBBAA keeps its alpha. Anything else is opaque black.
function parseRGBA(s: string): RGBA {
	const opaqueBlack: RGBA = [0, 0, 0, 255];
	if ((s.length !== 7 && s.length !== 9) || s[0] !== "#") return opaqueBlack;
	const n = (s.length - 1) / 2;
	const out: RGBA = [0, 0, 0, 255];
	for (let i = 0; i < n; i++) {
		const hi = hexVal(s.charCodeAt(1 + 2 * i));
		const lo = hexVal(s.charCodeAt(2 + 2 * i));
		if (hi < 0 || lo < 0) return opaqueBlack;
		out[i] = (hi << 4) | lo;
	}
	return out;
}

function hexVal(c: number): number {
	if (c >= 48 && c <= 57) return c - 48;
	if (c >= 97 && c <= 102) return c - 97 + 10;
	if (c >= 65 && c <= 70) return c - 65 + 10;
	return -1;
}
