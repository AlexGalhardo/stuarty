import type { LayerBox } from "./boxes";
import type { Point } from "./viewport";

// Hit testing over the boxes of the read model (display only). A rotated layer is hit through the inverse
// rotation about its centre.

export function containsPoint(box: LayerBox, point: Point): boolean {
	const cx = box.x + box.w / 2;
	const cy = box.y + box.h / 2;
	const angle = (-box.rotation * Math.PI) / 180;
	const dx = point.x - cx;
	const dy = point.y - cy;
	const lx = dx * Math.cos(angle) - dy * Math.sin(angle);
	const ly = dx * Math.sin(angle) + dy * Math.cos(angle);
	return Math.abs(lx) <= box.w / 2 && Math.abs(ly) <= box.h / 2;
}

// The topmost visible layer that holds the point. A locked layer is found too, so a click on it selects it and
// the inspector can unlock it; a group is never the target of a click, its children are.
export function hitTest(
	boxes: readonly LayerBox[],
	point: Point,
): LayerBox | undefined {
	for (let i = boxes.length - 1; i >= 0; i--) {
		const box = boxes[i];
		if (
			box !== undefined &&
			box.kind !== "group" &&
			box.visible &&
			containsPoint(box, point)
		) {
			return box;
		}
	}
	return undefined;
}
