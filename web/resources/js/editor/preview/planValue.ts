import type { PerFrame, PlanLayer, PlanMask, PlanWindow } from "../api/types";
import type { LayerGeometry } from "./types";

// A plan value is one number, or one number per frame from `first` (SDD-004
// section 10). The plan is the core's answer; this only indexes it.
export function valueAt(value: PerFrame, frame: number, first: number): number {
	if (typeof value === "number") return value;
	const at = Math.min(Math.max(frame - first, 0), value.length - 1);
	return value[at] ?? 0;
}

export function isActive(
	layer: { first: number; last: number },
	frame: number,
): boolean {
	return frame >= layer.first && frame <= layer.last;
}

// The layers drawn at a frame, in plan order (bottom to top).
export function layersAt(plan: PlanWindow, frame: number): PlanLayer[] {
	// A group layer (a composition drawn as one unit) is drawn by the server: the picture of a paused frame
	// comes from it (SDD-004 section 3.16), and the executors draw what they can compose themselves.
	return plan.layers.filter(
		(layer) => (layer.kind as string) !== "group" && isActive(layer, frame),
	);
}

export function geometryOf(layer: PlanLayer, frame: number): LayerGeometry {
	const first = layer.first;
	return {
		item: layer.item,
		cx: valueAt(layer.cx, frame, first),
		cy: valueAt(layer.cy, frame, first),
		w: valueAt(layer.w, frame, first),
		h: valueAt(layer.h, frame, first),
		rotation: valueAt(layer.rotation, frame, first),
		scale: valueAt(layer.scale, frame, first),
		opacity: valueAt(layer.opacity, frame, first),
	};
}

// The sign each axis of the layer takes for the mirror of SDD-004 section 3.10:
// -1 turns the picture around the centre of the unrotated rectangle.
export function mirrorSigns(layer: Pick<PlanLayer, "flip_x" | "flip_y">): {
	x: 1 | -1;
	y: 1 | -1;
} {
	return {
		x: layer.flip_x === true ? -1 : 1,
		y: layer.flip_y === true ? -1 : 1,
	};
}

// The key of the bitmap a bitmap layer shows at a frame.
export function bitmapKeyAt(
	layer: Extract<PlanLayer, { kind: "bitmap" }>,
	frame: number,
): string | undefined {
	const at = Math.min(
		Math.max(frame - layer.first, 0),
		layer.bitmaps.index.length - 1,
	);
	return layer.bitmaps.keys[layer.bitmaps.index[at] ?? 0];
}

// The mask of a layer at a frame: the entry of `masks` when the plan carries
// one per frame (an animated mask property), else the constant `mask`.
export function maskAt(layer: PlanLayer, frame: number): PlanMask | null {
	const series = layer.masks;
	if (series === undefined || series.length === 0) return layer.mask ?? null;
	const at = Math.min(Math.max(frame - layer.first, 0), series.length - 1);
	return series[at] ?? null;
}

// The crop of a media layer at a frame: the entry of `crops` when the plan
// carries one per frame (an animated crop side), else the constant `crop`.
export function cropAt(
	layer: PlanLayer,
	frame: number,
): [number, number, number, number] | null {
	if (layer.kind !== "media") return null;
	const series = layer.crops;
	if (series === undefined || series.length === 0) return layer.crop;
	const at = Math.min(Math.max(frame - layer.first, 0), series.length - 1);
	return series[at] ?? layer.crop;
}

// The colour matrix of a layer at a frame: the entry of `color_matrices` when
// the plan carries one per frame (animated colour, SDD-004 section 3.5), else
// the constant `color_matrix`.
export function colorMatrixAt(
	layer: PlanLayer,
	frame: number,
): number[] | null {
	if (layer.kind !== "media") return null;
	const series = layer.color_matrices;
	if (series === undefined || series.length === 0) return layer.color_matrix;
	const at = Math.min(Math.max(frame - layer.first, 0), series.length - 1);
	return series[at] ?? null;
}
