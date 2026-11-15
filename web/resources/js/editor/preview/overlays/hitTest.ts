import type { LayerGeometry } from "../types";

// A point in project pixels, as the canvas box is drawn (SDD-012 section 11.6).
export interface Point {
	x: number;
	y: number;
}

// Whether the point lies inside the rectangle of a layer: centre `cx, cy`, size
// `w, h`, rotated by `rotation` degrees clockwise about the centre. Screen
// arithmetic on the geometry of the plan; it decides nothing about the edit.
export function containsPoint(layer: LayerGeometry, point: Point): boolean {
	const dx = point.x - layer.cx;
	const dy = point.y - layer.cy;
	const angle = (layer.rotation * Math.PI) / 180;
	const cos = Math.cos(angle);
	const sin = Math.sin(angle);
	const x = dx * cos + dy * sin;
	const y = -dx * sin + dy * cos;
	return Math.abs(x) <= layer.w / 2 && Math.abs(y) <= layer.h / 2;
}

// The item of the topmost layer under the point: the last of the plan order
// (bottom to top) whose rotated rectangle contains it, else null.
export function hitTest(
	layers: readonly LayerGeometry[],
	point: Point,
): string | null {
	for (let i = layers.length - 1; i >= 0; i--) {
		const layer = layers[i];
		if (layer !== undefined && containsPoint(layer, point)) return layer.item;
	}
	return null;
}
