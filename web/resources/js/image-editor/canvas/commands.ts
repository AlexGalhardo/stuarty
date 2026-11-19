import type { ImageOperation } from "../api/types";
import type { Box } from "./gestures";

// The command a gesture asks for (SDD-012 section 24.4): one `layer.transform` per layer that changed, naming
// only the fields that changed. The box is given in canvas pixels; a layer inside a group is sent in the
// coordinates of the group, so the offset of its groups is taken off. The server validates and applies.

export interface Moved {
	id: string;
	before: Box;
	after: Box;
	offsetX: number;
	offsetY: number;
	// A group has no size and no rotation of its own: only its offset moves.
	group: boolean;
}

export function transformOps(items: readonly Moved[]): ImageOperation[] {
	const ops: ImageOperation[] = [];
	for (const item of items) {
		const args: { [key: string]: unknown } = { layer_id: item.id };
		const { before, after } = item;
		if (after.x !== before.x) args.x = after.x - item.offsetX;
		if (after.y !== before.y) args.y = after.y - item.offsetY;
		if (!item.group) {
			if (after.w !== before.w) args.w = after.w;
			if (after.h !== before.h) args.h = after.h;
			if (after.rotation !== before.rotation) args.rotation = after.rotation;
		}
		if (Object.keys(args).length > 1) ops.push({ op: "layer.transform", args });
	}
	return ops;
}
