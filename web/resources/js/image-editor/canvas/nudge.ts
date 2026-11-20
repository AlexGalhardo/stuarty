import type { ImageOperation } from "../api/types";
import { findLayer } from "../state/context";
import type { ImageState } from "../state/store";

// The command of an arrow key: each selected, unlocked layer moves by whole pixels, in the coordinates of its
// parent, so a layer of a group is moved by the same distance on the canvas.
export function nudgeOps(
	state: Pick<ImageState, "doc" | "selection">,
	dx: number,
	dy: number,
): ImageOperation[] {
	const doc = state.doc;
	if (doc === null) return [];
	const ops: ImageOperation[] = [];
	for (const id of state.selection) {
		const layer = findLayer(doc.layers, id);
		if (layer === undefined || layer.locked === true) continue;
		ops.push({
			op: "layer.transform",
			args: { layer_id: id, x: layer.x + dx, y: layer.y + dy },
		});
	}
	return ops;
}
