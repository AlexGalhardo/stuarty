// The image measures of preview-vs-export (SDD-012 section 20.3, NFR-011). They
// run in the Node runner of the test, never in the app: the browser only hands
// over decoded pixels.

export interface Pixels {
	width: number;
	height: number;
	// RGBA, 4 bytes a pixel, row by row.
	data: Uint8Array | Uint8ClampedArray;
}

// Rec. 601 luma, the weights the browser and FFmpeg use for sRGB.
export function luma({ width, height, data }: Pixels): Float64Array {
	const y = new Float64Array(width * height);
	for (let i = 0; i < y.length; i++) {
		y[i] =
			0.299 * data[i * 4] + 0.587 * data[i * 4 + 1] + 0.114 * data[i * 4 + 2];
	}
	return y;
}

// Mean SSIM of the luma over 8x8 blocks, stride 4 (Wang et al., K1 0.01, K2 0.03).
export function lumaSsim(a: Pixels, b: Pixels): number {
	if (a.width !== b.width || a.height !== b.height) {
		throw new Error(
			`ssim of ${a.width}x${a.height} against ${b.width}x${b.height}`,
		);
	}
	const w = a.width;
	const h = a.height;
	const x = luma(a);
	const y = luma(b);
	const c1 = (0.01 * 255) ** 2;
	const c2 = (0.03 * 255) ** 2;
	let sum = 0;
	let n = 0;
	for (let by = 0; by + 8 <= h; by += 4) {
		for (let bx = 0; bx + 8 <= w; bx += 4) {
			let mx = 0;
			let my = 0;
			for (let j = 0; j < 8; j++) {
				for (let i = 0; i < 8; i++) {
					mx += x[(by + j) * w + bx + i];
					my += y[(by + j) * w + bx + i];
				}
			}
			mx /= 64;
			my /= 64;
			let vx = 0;
			let vy = 0;
			let cv = 0;
			for (let j = 0; j < 8; j++) {
				for (let i = 0; i < 8; i++) {
					const dx = x[(by + j) * w + bx + i] - mx;
					const dy = y[(by + j) * w + bx + i] - my;
					vx += dx * dx;
					vy += dy * dy;
					cv += dx * dy;
				}
			}
			vx /= 63;
			vy /= 63;
			cv /= 63;
			sum +=
				((2 * mx * my + c1) * (2 * cv + c2)) /
				((mx * mx + my * my + c1) * (vx + vy + c2));
			n++;
		}
	}
	return sum / n;
}

export interface Centroid {
	x: number;
	y: number;
	count: number;
}

// The centroid of the pixels that `hit` accepts, over the rows from `top` up to
// (not including) `bottom`.
export function centroid(
	{ width, height, data }: Pixels,
	hit: (r: number, g: number, b: number) => boolean,
	top = 0,
	bottom = height,
): Centroid {
	let sx = 0;
	let sy = 0;
	let count = 0;
	for (let row = top; row < bottom; row++) {
		for (let col = 0; col < width; col++) {
			const i = (row * width + col) * 4;
			if (hit(data[i], data[i + 1], data[i + 2])) {
				sx += col;
				sy += row;
				count++;
			}
		}
	}
	return { x: sx / count, y: sy / count, count };
}

// The offset, in pixels, that brings `b` onto `a` (b sampled at p + offset
// equals a at p), found by the Lucas-Kanade method on the luma: a few
// Gauss-Newton steps of a single translation over the interior. It follows the
// edges and so stays exact for a blurred or filtered picture, where a search
// over integer shifts has no sharp minimum. It is the displacement of the whole
// picture, which stands for that of its layers where a case has no marker of its
// own. A picture without any edge gives 0.
export function globalShift(
	a: Pixels,
	b: Pixels,
	border = Math.round(Math.min(a.width, a.height) * 0.1),
): { dx: number; dy: number; distance: number; texture: number } {
	const w = a.width;
	const h = a.height;
	const x = luma(a);
	const y = luma(b);
	// The border is left out: a blurred layer fades to transparent past its edge
	// in both pictures but not by the same law.
	const margin = Math.max(border, 6);
	const sample = (px: number, py: number): number => {
		const x0 = Math.floor(px);
		const y0 = Math.floor(py);
		const fx = px - x0;
		const fy = py - y0;
		const v = (cx: number, cy: number): number =>
			y[
				Math.min(Math.max(cy, 0), h - 1) * w + Math.min(Math.max(cx, 0), w - 1)
			];
		return (
			v(x0, y0) * (1 - fx) * (1 - fy) +
			v(x0 + 1, y0) * fx * (1 - fy) +
			v(x0, y0 + 1) * (1 - fx) * fy +
			v(x0 + 1, y0 + 1) * fx * fy
		);
	};
	let dx = 0;
	let dy = 0;
	let texture = 0;
	for (let step = 0; step < 8; step++) {
		let gxx = 0;
		let gxy = 0;
		let gyy = 0;
		let bx = 0;
		let by = 0;
		for (let row = margin; row < h - margin; row++) {
			for (let col = margin; col < w - margin; col++) {
				const px = col + dx;
				const py = row + dy;
				const ix = (sample(px + 1, py) - sample(px - 1, py)) / 2;
				const iy = (sample(px, py + 1) - sample(px, py - 1)) / 2;
				const e = sample(px, py) - x[row * w + col];
				gxx += ix * ix;
				gxy += ix * iy;
				gyy += iy * iy;
				bx += ix * e;
				by += iy * e;
			}
		}
		const det = gxx * gyy - gxy * gxy;
		// The smaller eigenvalue of the structure tensor per pixel: how much edge
		// the picture has in its weakest direction. Under about 0.05 the
		// offset is noise.
		const half = (gxx + gyy) / 2;
		texture =
			(half - Math.sqrt(Math.max(half * half - det, 0))) /
			((w - 2 * margin) * (h - 2 * margin));
		if (det < 1e-6) break;
		const ux = -(gyy * bx - gxy * by) / det;
		const uy = -(gxx * by - gxy * bx) / det;
		dx += ux;
		dy += uy;
		if (Math.hypot(ux, uy) < 0.001) break;
	}
	return { dx, dy, distance: Math.hypot(dx, dy), texture };
}

// The centroid of how much a pixel leans to red (or blue) over the other two
// channels, weighted by that excess, so a marker that is blurred or half
// transparent still has a centre. The weights of the two pictures are summed
// the same way, so equal pictures give equal centroids.
export function chromaCentroid(
	{ width, height, data }: Pixels,
	channel: "red" | "blue",
): { x: number; y: number; mass: number } {
	let sx = 0;
	let sy = 0;
	let mass = 0;
	for (let i = 0; i < width * height; i++) {
		const r = data[i * 4];
		const g = data[i * 4 + 1];
		const b = data[i * 4 + 2];
		const excess = channel === "red" ? r - (g + b) / 2 : b - (r + g) / 2;
		if (excess > 8) {
			sx += (i % width) * excess;
			sy += Math.floor(i / width) * excess;
			mass += excess;
		}
	}
	return { x: sx / mass, y: sy / mass, mass };
}
