import type { EditRequest } from "../api/types";

// Builds the operations of the gestures of SDD-012 section 8.2 from values the
// user chose. It requests; the core decides (rules T5 and T6). On the
// allowlist of rule R5: gating compares the playhead with an item's range.

export type Operations = EditRequest["operations"];

export interface TrackRef {
	id: string;
	kind: "visual" | "audio";
}

export interface ItemRange {
	id: string;
	start: number;
	duration: number;
}

// Rule T8: every time argument carries a unit.
export const frames = (n: number): string => `${n}f`;

// Whether the playhead lies strictly inside the item (SDD-012 section 8.3);
// an affordance, never a prediction of success.
export function containsStrictly(item: ItemRange, playhead: number): boolean {
	return playhead > item.start && playhead < item.start + item.duration;
}

// "Add to timeline": on the first visual track by order; when there is none,
// the same batch adds one first.
export function addClipOperations(input: {
	assetId: string;
	playhead: number;
	firstVisualTrack: string | null;
	newTrackId: () => string;
}): Operations {
	const ops: Operations = [];
	let track = input.firstVisualTrack;
	if (track === null) {
		track = input.newTrackId();
		ops.push({ op: "track.add", args: { kind: "visual", id: track } });
	}
	ops.push({
		op: "clip.add",
		args: {
			track_id: track,
			asset_id: input.assetId,
			start: frames(input.playhead),
		},
	});
	return ops;
}

// One `item.split` per item whose range contains the playhead strictly.
export function splitOperations(
	items: readonly ItemRange[],
	playhead: number,
): Operations {
	return items
		.filter((item) => containsStrictly(item, playhead))
		.map((item) => ({
			op: "item.split",
			args: { item_id: item.id, at: frames(playhead) },
		}));
}

// One `item.trim` per item that the playhead lies strictly inside.
export function trimOperations(
	items: readonly ItemRange[],
	edge: "start" | "end",
	playhead: number,
): Operations {
	return items
		.filter((item) => containsStrictly(item, playhead))
		.map((item) => ({
			op: "item.trim",
			args: { item_id: item.id, edge, to: frames(playhead) },
		}));
}

// The Add caption dialog: text and a duration the user typed with its unit.
export function captionOperations(input: {
	text: string;
	duration: string;
	playhead: number;
	firstVisualTrack: string | null;
	newTrackId: () => string;
}): Operations {
	const ops: Operations = [];
	let track = input.firstVisualTrack;
	if (track === null) {
		track = input.newTrackId();
		ops.push({ op: "track.add", args: { kind: "visual", id: track } });
	}
	ops.push({
		op: "caption.add",
		args: {
			track_id: track,
			start: frames(input.playhead),
			text: input.text,
			duration: input.duration,
		},
	});
	return ops;
}

// The inspector fields of section 12.1: the text of a time field is already
// a string with a unit (rule T8), checked by `normalizeTimeInput`.
export function renameOperations(itemId: string, name: string): Operations {
	return [{ op: "item.rename", args: { item_id: itemId, name } }];
}

export function moveOperations(itemId: string, start: string): Operations {
	return [{ op: "item.move", args: { item_id: itemId, start } }];
}

// Alt+Left and Alt+Right: one `item.move` per selected item, `start` moved by
// `delta` frames and never below frame 0. One batch, one revision. The core
// applies the operations in order, so the item that leads the move goes first
// (the last one when moving right, the first when moving left): neighbours that
// move together never meet in the middle of the batch.
export function nudgeOperations(
	items: readonly ItemRange[],
	delta: number,
): Operations {
	const ordered = [...items].sort((a, b) =>
		delta > 0 ? b.start - a.start : a.start - b.start,
	);
	return ordered.map((item) => ({
		op: "item.move",
		args: { item_id: item.id, start: frames(Math.max(0, item.start + delta)) },
	}));
}

// The track `sign` places away from `trackId` in display order, else null.
export function adjacentTrack(
	trackIds: readonly string[],
	trackId: string,
	sign: 1 | -1,
): string | null {
	const at = trackIds.indexOf(trackId);
	if (at < 0) return null;
	return trackIds[at + sign] ?? null;
}

// Alt+Up and Alt+Down: each item to the track the caller picked for it.
export function moveToTrackOperations(
	moves: readonly { itemId: string; trackId: string }[],
): Operations {
	return moves.map((move) => ({
		op: "item.move",
		args: { item_id: move.itemId, track_id: move.trackId },
	}));
}

export function trimEndOperations(itemId: string, to: string): Operations {
	return [{ op: "item.trim", args: { item_id: itemId, edge: "end", to } }];
}

// The same operations with the `start` the core named as the next free slot
// (`details.next_free`, a frame number as received).
export function withNextFree(ops: Operations, nextFree: number): Operations {
	return ops.map((operation) =>
		operation.op === "clip.add" ||
		operation.op === "caption.add" ||
		operation.op === "item.move"
			? {
					...operation,
					args: { ...operation.args, start: frames(nextFree) },
				}
			: operation,
	);
}
