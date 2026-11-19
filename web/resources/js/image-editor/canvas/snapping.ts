import type { LayerBox } from "./boxes";

// The pointer snaps to the edges and centres that the read model describes (SDD-012 section 24.4): the canvas
// and the other visible layers. The tolerance is in screen pixels. What is sent is the integer position that
// results.

export const SNAP_TOLERANCE = 6;

export interface Lines {
	xs: number[];
	ys: number[];
}

export interface Snap {
	// The shift that puts the nearest edge or centre on its target; 0 when none is within the tolerance.
	dx: number;
	dy: number;
	// The target lines to draw, in canvas pixels.
	guideX: number | null;
	guideY: number | null;
}

export function snapTargets(
	boxes: readonly LayerBox[],
	canvas: { w: number; h: number },
	skip: ReadonlySet<string>,
): Lines {
	const xs = [0, canvas.w / 2, canvas.w];
	const ys = [0, canvas.h / 2, canvas.h];
	for (const box of boxes) {
		if (skip.has(box.id) || !box.visible || box.kind === "group") continue;
		if (box.rotation !== 0) continue;
		xs.push(box.x, box.x + box.w / 2, box.x + box.w);
		ys.push(box.y, box.y + box.h / 2, box.y + box.h);
	}
	return { xs, ys };
}

function nearest(
	candidates: readonly number[],
	targets: readonly number[],
	tolerance: number,
): { shift: number; line: number } | null {
	let best: { shift: number; line: number } | null = null;
	for (const c of candidates) {
		for (const t of targets) {
			const shift = t - c;
			if (
				Math.abs(shift) <= tolerance &&
				(best === null || Math.abs(shift) < Math.abs(best.shift))
			) {
				best = { shift, line: t };
			}
		}
	}
	return best;
}

// `candidates` are the edges and the centre of the moving box; `zoom` turns the screen tolerance into canvas
// pixels.
export function snap(candidates: Lines, targets: Lines, zoom: number): Snap {
	const tolerance = SNAP_TOLERANCE / zoom;
	const x = nearest(candidates.xs, targets.xs, tolerance);
	const y = nearest(candidates.ys, targets.ys, tolerance);
	return {
		dx: x?.shift ?? 0,
		dy: y?.shift ?? 0,
		guideX: x?.line ?? null,
		guideY: y?.line ?? null,
	};
}

export function edgesOf(box: {
	x: number;
	y: number;
	w: number;
	h: number;
}): Lines {
	return {
		xs: [box.x, box.x + box.w / 2, box.x + box.w],
		ys: [box.y, box.y + box.h / 2, box.y + box.h],
	};
}
