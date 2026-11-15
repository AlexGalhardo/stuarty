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

// "Add to timeline" on an audio card: `audio.add` on the first audio track by
// order; when there is none, the same batch adds one first with an id the UI
// supplies.
export function addAudioOperations(input: {
	assetId: string;
	playhead: number;
	firstAudioTrack: string | null;
	newTrackId: () => string;
}): Operations {
	const ops: Operations = [];
	let track = input.firstAudioTrack;
	if (track === null) {
		track = input.newTrackId();
		ops.push({ op: "track.add", args: { kind: "audio", id: track } });
	}
	ops.push({
		op: "audio.add",
		args: {
			track_id: track,
			asset_id: input.assetId,
			start: frames(input.playhead),
		},
	});
	return ops;
}

// A library card released on a row: the operation follows the asset and the row
// (audio assets and audio rows take `audio.add`), the track is the row under the
// pointer and `start` the frame under it. The core judges the kind and the place.
export function dropAssetOperations(input: {
	assetId: string;
	assetKind: "video" | "audio" | "image";
	trackId: string;
	trackKind: "visual" | "audio";
	start: number;
}): Operations {
	const op =
		input.assetKind === "audio" || input.trackKind === "audio"
			? "audio.add"
			: "clip.add";
	return [
		{
			op,
			args: {
				track_id: input.trackId,
				asset_id: input.assetId,
				start: frames(input.start),
			},
		},
	];
}

// "Remove from project" on a card; the core refuses with E_IN_USE while an item
// still uses the asset.
export function removeAssetOperations(assetId: string): Operations {
	return [{ op: "asset.remove", args: { asset_id: assetId } }];
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

export type Ripple = "track" | "all";

// Delete, Shift+Delete and the menu entries (SDD-012 section 8.2): one
// `item.delete` for the whole selection; the ripple the person chose is only
// passed on, the core moves what follows.
export function deleteOperations(
	itemIds: readonly string[],
	ripple?: Ripple,
): Operations {
	if (itemIds.length === 0) return [];
	return [
		{
			op: "item.delete",
			args: {
				item_ids: [...itemIds],
				...(ripple === undefined ? {} : { ripple }),
			},
		},
	];
}

// Ctrl+D: a copy of each item right after its end, on its own track. The end
// frame is read from the item for request composition only; the core judges
// the overlap. The copies go in start order.
export function duplicateOperations(items: readonly ItemRange[]): Operations {
	return [...items]
		.sort((a, b) => a.start - b.start)
		.map((item) => ({
			op: "item.duplicate",
			args: { item_id: item.id, start: frames(item.start + item.duration) },
		}));
}

// The gap menu: the frame under the pointer names the gap, the core measures it.
export function closeGapOperations(trackId: string, at: number): Operations {
	return [
		{ op: "timeline.close_gap", args: { track_id: trackId, at: frames(at) } },
	];
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

export interface MoveTarget {
	id: string;
	from: number;
	to: number;
	// Set when the item goes to another track.
	trackId?: string;
}

// A drag release: one `item.move` per item that changed, `start` only when it
// changed, `track_id` only when the track did (SDD-012 section 8.2). The
// operations are requests built from the ghost; the core decides. They are
// ordered so that items moving together never meet inside the batch: those
// moving right go first, the last one first, then those moving left.
export function moveItems(targets: readonly MoveTarget[]): Operations {
	const changed = targets.filter(
		(m) => m.to !== m.from || m.trackId !== undefined,
	);
	const right = changed
		.filter((m) => m.to > m.from)
		.sort((a, b) => b.from - a.from);
	const rest = changed
		.filter((m) => m.to <= m.from)
		.sort((a, b) => a.from - b.from);
	return [...right, ...rest].map((m) => ({
		op: "item.move",
		args: {
			item_id: m.id,
			...(m.trackId === undefined ? {} : { track_id: m.trackId }),
			...(m.to === m.from ? {} : { start: frames(m.to) }),
		},
	}));
}

// A trim handle released: one `item.trim` to the frame the ghost shows.
export function trimEdgeOperations(
	itemId: string,
	edge: "start" | "end",
	to: number,
): Operations {
	return [{ op: "item.trim", args: { item_id: itemId, edge, to: frames(to) } }];
}

export function trimEndOperations(itemId: string, to: string): Operations {
	return [{ op: "item.trim", args: { item_id: itemId, edge: "end", to } }];
}

// The same operations with the `start` the core named as the next free slot
// (`details.next_free`, a frame number as received).
export function withNextFree(ops: Operations, nextFree: number): Operations {
	return ops.map((operation) =>
		operation.op === "clip.add" ||
		operation.op === "audio.add" ||
		operation.op === "caption.add" ||
		operation.op === "item.move"
			? {
					...operation,
					args: { ...operation.args, start: frames(nextFree) },
				}
			: operation.op === "item.trim"
				? {
						...operation,
						args: { ...operation.args, to: frames(nextFree) },
					}
				: operation,
	);
}

export type TransformProperty = "x" | "y" | "scale" | "rotation" | "opacity";
export type TransformChanges = Partial<Record<TransformProperty, number>>;

const TRANSFORM_PROPERTIES: readonly TransformProperty[] = [
	"x",
	"y",
	"scale",
	"rotation",
	"opacity",
];

export interface TransformTarget {
	id: string;
	// The properties whose key exists in the item's `keyframes` (gating of
	// SDD-012 section 8.3: only whether the key exists).
	animated: readonly string[];
}

// A transform released on the canvas or committed in the inspector: one
// `item.set_transform` per item with the changed fields that have no keyframes,
// and one `keyframe.set` at the playhead per changed field that has them. The
// values are what the user chose; the core judges them.
export function transformOperations(
	targets: readonly TransformTarget[],
	changes: TransformChanges,
	playhead: number,
): Operations {
	const ops: Operations = [];
	for (const target of targets) {
		const stat: TransformChanges = {};
		const keyed: [TransformProperty, number][] = [];
		for (const property of TRANSFORM_PROPERTIES) {
			const value = changes[property];
			if (value === undefined) continue;
			if (target.animated.includes(property)) keyed.push([property, value]);
			else stat[property] = value;
		}
		if (Object.keys(stat).length > 0) {
			ops.push({
				op: "item.set_transform",
				args: { item_id: target.id, ...stat },
			});
		}
		for (const [property, value] of keyed) {
			ops.push({
				op: "keyframe.set",
				args: {
					item_id: target.id,
					property,
					at: frames(playhead),
					value,
				},
			});
		}
	}
	return ops;
}

// E_INVALID_STATE on a transform ("use keyframes"): the same gesture resent as
// one `keyframe.set` per property at the playhead.
export function keyframeFallbackOperations(
	ops: Operations,
	playhead: number,
): Operations {
	return ops.flatMap((operation) => {
		if (operation.op !== "item.set_transform") return [operation];
		const { item_id: itemId, ...values } = operation.args;
		return TRANSFORM_PROPERTIES.flatMap((property) => {
			const value = values[property];
			return typeof value === "number"
				? [
						{
							op: "keyframe.set",
							args: {
								item_id: itemId,
								property,
								at: frames(playhead),
								value,
							},
						},
					]
				: [];
		});
	});
}
