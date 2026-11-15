import type { PerFrame, PlanLayer, PlanWindow } from "../api/types";
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
	return plan.layers.filter((layer) => isActive(layer, frame));
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
