import type { CaptionStyle, Word } from "@/generated/timeline";
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

// Toolbar "Add text" and `T`: a text item of 3 s at the playhead on the first
// visual track; when there is none, the same batch adds one first.
export function textAddOperations(input: {
	text: string;
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
		op: "text.add",
		args: {
			track_id: track,
			start: frames(input.playhead),
			duration: "3s",
			text: input.text,
		},
	});
	return ops;
}

// The inline editor of a text item and the content field of the inspector.
export function textSetOperations(itemId: string, text: string): Operations {
	return [{ op: "text.set", args: { item_id: itemId, text } }];
}

export type StyleValue = string | number | boolean;
export type StyleChanges = Record<string, StyleValue>;

// One style field of a text item: the partial `style` holds only the changed key.
export function textStyleOperations(
	itemId: string,
	style: StyleChanges,
): Operations {
	return [
		{
			op: "text.set",
			args: { item_id: itemId, style: style === null ? null : { ...style } },
		},
	];
}

// The caption word operations of SDD-012 section 8.2: the arguments are what the
// person chose; the core judges them.
export function captionEditWordOperations(
	itemId: string,
	index: number,
	text: string,
): Operations {
	return [{ op: "caption.edit_word", args: { item_id: itemId, index, text } }];
}

export function captionSplitOperations(
	itemId: string,
	beforeWord: number,
): Operations {
	return [
		{
			op: "caption.split",
			args: { item_id: itemId, before_word: beforeWord },
		},
	];
}

export function captionMergeOperations(itemIds: readonly string[]): Operations {
	return [{ op: "caption.merge", args: { item_ids: [...itemIds] } }];
}

// "Generate" of the auto captions panel (SDD-012 section 12.4): the captions of
// the clip's transcript on `trackId`, or on a track named Captions that the same
// batch adds first. The core fills the words from the transcript.
export function captionFromTranscriptOperations(input: {
	clipId: string;
	trackId: string | null;
	newTrackId: () => string;
	trackName: string;
	maxWords: number;
	maxChars: number;
	maxGap: string;
	replace: boolean;
}): Operations {
	const ops: Operations = [];
	let track = input.trackId;
	if (track === null) {
		track = input.newTrackId();
		ops.push({
			op: "track.add",
			args: { kind: "visual", id: track, name: input.trackName },
		});
	}
	ops.push({
		op: "caption.from_transcript",
		args: {
			clip_item_id: input.clipId,
			track_id: track,
			max_words: input.maxWords,
			max_chars: input.maxChars,
			max_gap: input.maxGap,
			replace: input.replace,
		},
	});
	return ops;
}

// "Re-sync captions": the captions linked to the clip follow its current start,
// in point and speed.
export function captionRetimeOperations(clipId: string): Operations {
	return [{ op: "caption.retime_from_clip", args: { clip_item_id: clipId } }];
}

export function captionBreaksOperations(
	itemId: string,
	breaks: readonly number[],
): Operations {
	return [
		{
			op: "caption.set_breaks",
			args: { item_id: itemId, breaks: [...breaks] },
		},
	];
}

// The breaks after the word at `index` starts or stops a line; the list stays
// strictly increasing.
export function toggleBreak(
	breaks: readonly number[],
	index: number,
): number[] {
	return breaks.includes(index)
		? breaks.filter((b) => b !== index)
		: [...breaks, index].sort((a, b) => a - b);
}

// `style: null` clears the override ("Use track style"); a style replaces it
// whole, so the caller passes the stored style with the changed key.
export function captionStyleOperations(
	itemId: string,
	style: CaptionStyle | null,
): Operations {
	return [
		{
			op: "caption.set_style",
			args: { item_id: itemId, style: style === null ? null : { ...style } },
		},
	];
}

// A word as `caption.set_words` takes it: times are strings with a unit.
export type WordArg = {
	text: string;
	start: string;
	duration: string;
};

// The words of the read model as arguments, so a request copies the words it
// does not change (request composition, SDD-012 section 8.2).
export function wordArgs(words: readonly Word[]): WordArg[] {
	return words.map((word) => ({
		text: word.text,
		start: frames(word.start),
		duration: frames(word.duration),
	}));
}

export function captionWordsOperations(
	itemId: string,
	words: readonly WordArg[],
): Operations {
	return [
		{ op: "caption.set_words", args: { item_id: itemId, words: [...words] } },
	];
}

// One field of one word replaced by what the person typed; the others are copied.
export function withWordField(
	words: readonly Word[],
	index: number,
	field: "start" | "duration",
	value: string,
): WordArg[] {
	return wordArgs(words).map((word, i) =>
		i === index ? { ...word, [field]: value } : word,
	);
}

// The frame (relative to the caption) a boundary may reach: inside the two
// words it separates, a frame from each far edge.
export function clampBoundary(
	words: readonly Word[],
	index: number,
	to: number,
): number {
	const before = words[index - 1];
	const after = words[index];
	if (before === undefined || after === undefined) return to;
	return Math.min(
		after.start + after.duration - 1,
		Math.max(before.start + 1, to),
	);
}

// Dragging the boundary in front of word `index` to frame `to` (relative to the
// caption): the two neighbouring words are adjusted, the others copied. Where
// the previous word ends at the boundary it follows it; where there is a gap it
// only gives way when the boundary passes its end.
export function boundaryWords(
	words: readonly Word[],
	index: number,
	to: number,
): WordArg[] {
	const before = words[index - 1];
	const after = words[index];
	if (before === undefined || after === undefined) return wordArgs(words);
	const afterEnd = after.start + after.duration;
	const beforeEnd = before.start + before.duration;
	const edge = clampBoundary(words, index, to);
	const newBeforeEnd =
		beforeEnd === after.start ? edge : Math.min(beforeEnd, edge);
	return wordArgs(words).map((word, i) => {
		if (i === index - 1) {
			return { ...word, duration: frames(newBeforeEnd - before.start) };
		}
		if (i === index) {
			return {
				...word,
				start: frames(edge),
				duration: frames(afterEnd - edge),
			};
		}
		return word;
	});
}

// The bulk edit: the words typed, one per whitespace-separated token, over the
// times of the stored words. Only the same count can keep the times, so a
// different count answers null and nothing is sent.
export function bulkEditWords(
	words: readonly Word[],
	text: string,
): WordArg[] | null {
	const typed = text.split(/\s+/).filter((token) => token !== "");
	if (typed.length !== words.length) return null;
	return wordArgs(words).map((word, i) => ({
		...word,
		text: typed[i] ?? word.text,
	}));
}

// The inspector fields of section 12.1: the text of a time field is already
// a string with a unit (rule T8), checked by `normalizeTimeInput`.
export function renameOperations(itemId: string, name: string): Operations {
	return [{ op: "item.rename", args: { item_id: itemId, name } }];
}

export type Ripple = "track" | "all";

export const KEYFRAME_PROPERTIES = [
	"x",
	"y",
	"scale",
	"rotation",
	"opacity",
	"volume",
] as const;
export type KeyframeProperty = (typeof KEYFRAME_PROPERTIES)[number];

// The spellings of the registry (`keyframe.set` easing).
export const KEYFRAME_EASINGS = [
	"linear",
	"ease_in",
	"ease_out",
	"ease_in_out",
	"hold",
] as const;
export type KeyframeEasing = (typeof KEYFRAME_EASINGS)[number];

// Whether the playhead lies inside the item, its end included: the one gate of
// "Add at playhead" (SDD-012 section 8.3), never a prediction of success.
export function containsInclusive(item: ItemRange, playhead: number): boolean {
	return playhead >= item.start && playhead <= item.start + item.duration;
}

// A keyframe is stored with `t` relative to the item start; the commands name it
// by its position on the timeline.
export const keyframeFrame = (itemStart: number, t: number): number =>
	itemStart + t;

// "Add at playhead" and the easing select of a row: the value and the easing are
// what the person chose; the core judges them.
export function keyframeSetOperations(input: {
	itemId: string;
	property: KeyframeProperty;
	frame: number;
	value: number;
	easing: KeyframeEasing;
}): Operations {
	return [
		{
			op: "keyframe.set",
			args: {
				item_id: input.itemId,
				property: input.property,
				at: frames(input.frame),
				value: input.value,
				easing: input.easing,
			},
		},
	];
}

export function keyframeRemoveOperations(
	itemId: string,
	property: KeyframeProperty,
	frame: number,
): Operations {
	return [
		{
			op: "keyframe.remove",
			args: { item_id: itemId, property, at: frames(frame) },
		},
	];
}

// One property, or every animated property of the item when none is named.
export function keyframeClearOperations(
	itemId: string,
	property?: KeyframeProperty,
): Operations {
	return [
		{
			op: "keyframe.clear",
			args:
				property === undefined
					? { item_id: itemId }
					: { item_id: itemId, property },
		},
	];
}

// The spellings of the registry (`item.set_transition`, `text.apply_preset`).
export const TRANSITION_KINDS = [
	"fade",
	"slide_left",
	"slide_right",
	"slide_up",
	"slide_down",
] as const;
export type TransitionKind = (typeof TRANSITION_KINDS)[number];

export const ANIMATION_PRESETS = [
	"fade_up",
	"fade_down",
	"pop",
	"zoom_in",
	"slide_left",
	"slide_right",
] as const;
export type AnimationPreset = (typeof ANIMATION_PRESETS)[number];

export type Edge = "in" | "out";

// The default length of a crossfade from the menu, which has no field for it.
export const CROSSFADE_DURATION = "0.5s";

// One transition control: the kind (null removes it) and, when the control has
// one, the duration, which is a time with its unit. The core judges whether it fits.
export function transitionOperations(input: {
	itemId: string;
	edge: Edge;
	kind: TransitionKind | null;
	duration?: string;
}): Operations {
	return [
		{
			op: "item.set_transition",
			args: {
				item_id: input.itemId,
				edge: input.edge,
				kind: input.kind,
				...(input.duration === undefined ? {} : { duration: input.duration }),
			},
		},
	];
}

export function presetOperations(input: {
	itemId: string;
	edge: Edge;
	preset: AnimationPreset;
	duration?: string;
}): Operations {
	return [
		{
			op: "text.apply_preset",
			args: {
				item_id: input.itemId,
				edge: input.edge,
				preset: input.preset,
				...(input.duration === undefined ? {} : { duration: input.duration }),
			},
		},
	];
}

// "Crossfade" with two items selected: `from` is the one whose stored start is
// smaller. Only the two stored numbers are compared, to name the arguments.
export function crossfadeOperations(
	a: ItemRange,
	b: ItemRange,
	duration: string = CROSSFADE_DURATION,
): Operations {
	const [from, to] = a.start <= b.start ? [a, b] : [b, a];
	return [
		{
			op: "item.crossfade",
			args: { from_item_id: from.id, to_item_id: to.id, duration },
		},
	];
}

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

// Where a track goes in the order: directly over or under another track, or at
// an end. The core computes the new `order` (SDD-002 C.6 and C.8).
export type TrackPlacement =
	| { above: string }
	| { below: string }
	| { position: "top" | "bottom" };

// The header menu "Add track above or below": the new track sits next to the
// track whose menu was used.
export function trackAddOperations(
	kind: "visual" | "audio",
	placement: TrackPlacement,
): Operations {
	return [{ op: "track.add", args: { kind, ...placement } }];
}

export interface TrackChanges {
	name?: string;
	hidden?: boolean;
	muted?: boolean;
}

export function trackUpdateOperations(
	trackId: string,
	changes: TrackChanges,
): Operations {
	return [{ op: "track.update", args: { track_id: trackId, ...changes } }];
}

export function trackMoveOperations(
	trackId: string,
	placement: TrackPlacement,
): Operations {
	return [{ op: "track.move", args: { track_id: trackId, ...placement } }];
}

// The first request of a removal has no `with_items`; after E_NOT_EMPTY and the
// confirmation the same operation is sent with it.
export function trackRemoveOperations(
	trackId: string,
	withItems: boolean,
): Operations {
	return [
		{
			op: "track.remove",
			args: withItems
				? { track_id: trackId, with_items: true }
				: { track_id: trackId },
		},
	];
}

// A partial style merges into the track's style unless `replace` is set, in
// which case it becomes the whole style (SDD-002 C.10).
export function trackCaptionStyleOperations(
	trackId: string,
	style: StyleChanges,
	replace: boolean,
): Operations {
	return [
		{
			op: "track.set_caption_style",
			args: replace
				? { track_id: trackId, style, replace: true }
				: { track_id: trackId, style },
		},
	];
}

// The size presets of RF-002 as the project panel offers them. The sizes are
// the core's (service/projects.go); the panel sends them as `settings.set`.
export const SIZE_PRESETS = [
	{ id: "9:16", width: 1080, height: 1920 },
	{ id: "1:1", width: 1080, height: 1080 },
	{ id: "16:9", width: 1920, height: 1080 },
	{ id: "4:5", width: 1080, height: 1350 },
] as const;
export type SizePresetId = (typeof SIZE_PRESETS)[number]["id"];

export interface SettingsChanges {
	width?: number;
	height?: number;
	background?: string;
	loudness_target?: number;
}

export function settingsSetOperations(changes: SettingsChanges): Operations {
	return [{ op: "settings.set", args: { ...changes } }];
}

// The frame rates of the select, as `settings.set_fps` takes them: an integer
// or a rational with denominator 1001 (SDD-002 C.3).
export const FRAME_RATES = [
	"24",
	"25",
	"30",
	"50",
	"60",
	"24000/1001",
	"30000/1001",
	"60000/1001",
] as const;

export function settingsFpsOperations(fps: string): Operations {
	return [{ op: "settings.set_fps", args: { fps } }];
}

export function projectRenameOperations(name: string): Operations {
	return [{ op: "project.rename", args: { name } }];
}
