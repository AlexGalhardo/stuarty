import type { CaptionStyle, Item, Word } from "@/generated/timeline";
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
	track?: string;
}

// Rule T8: every time argument carries a unit.
export const frames = (n: number): string => `${n}f`;

// Whether the playhead lies strictly inside the item (SDD-012 section 8.3);
// an affordance, never a prediction of success.
export function containsStrictly(item: ItemRange, playhead: number): boolean {
	return playhead > item.start && playhead < item.start + item.duration;
}

// "Add to timeline": on the first visual track by order, after the last item
// (`start` omitted, SDD-012 section 8.2); when there is no track, the same
// batch adds one first.
export function addClipOperations(input: {
	assetId: string;
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
		},
	});
	return ops;
}

// "Add to timeline" on an audio card: `audio.add` on the first audio track by
// order; when there is none, the same batch adds one first with an id the UI
// supplies.
export function addAudioOperations(input: {
	assetId: string;
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

// One `item.freeze_frame` for the first item the playhead lies strictly inside:
// the core holds the picture of that frame as a still after the split. It
// refuses an item that is not a clip of a video asset.
export function freezeFrameOperations(
	items: readonly ItemRange[],
	playhead: number,
): Operations {
	const item = items.find((candidate) => containsStrictly(candidate, playhead));
	return item === undefined
		? []
		: [
				{
					op: "item.freeze_frame",
					args: { item_id: item.id, at: frames(playhead) },
				},
			];
}

// One `item.repeat`: `copies` extra copies after the item, or the sequence
// cut at `until` (a time string). The core judges the rest.
export function repeatOperations(
	itemId: string,
	input: {
		copies?: number;
		until?: string;
		ripple?: "none" | "track";
		link?: boolean;
	},
): Operations {
	return [
		{
			op: "item.repeat",
			args: {
				item_id: itemId,
				...(input.copies === undefined ? {} : { copies: input.copies }),
				...(input.until === undefined ? {} : { until: input.until }),
				...(input.ripple === undefined ? {} : { ripple: input.ripple }),
				...(input.link === true ? { link: true } : {}),
			},
		},
	];
}

// One `item.trim` per item that the playhead lies strictly inside. `ripple` is
// omitted unless the caller names it: the core default stays none.
export function trimOperations(
	items: readonly ItemRange[],
	edge: "start" | "end",
	playhead: number,
	ripple?: "none" | "track",
): Operations {
	return items
		.filter((item) => containsStrictly(item, playhead))
		.map((item) => ({
			op: "item.trim",
			args: {
				item_id: item.id,
				edge,
				to: frames(playhead),
				...(ripple === undefined ? {} : { ripple }),
			},
		}));
}

// The Add caption dialog: text and a duration the user typed with its unit.
export function captionOperations(input: {
	text: string;
	duration: string;
	playhead: number;
}): Operations {
	return [
		{
			op: "caption.add",
			args: {
				start: frames(input.playhead),
				text: input.text,
				duration: input.duration,
			},
		},
	];
}

// Toolbar "Add text" and `T`: a text item of 3 s at the playhead. The core picks
// the track: the highest free visual one, or a new one on top (SDD-002 C.6).
export function textAddOperations(input: {
	text: string;
	playhead: number;
}): Operations {
	return [
		{
			op: "text.add",
			args: {
				start: frames(input.playhead),
				duration: "3s",
				text: input.text,
			},
		},
	];
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

export type SubtitleFormat = "srt" | "vtt";

// The format a file name says (`.srt`, `.vtt`), or none: the core then reads a
// WEBVTT header, else SRT.
export function subtitleFormatOf(fileName: string): SubtitleFormat | undefined {
	const name = fileName.toLowerCase();
	if (name.endsWith(".srt")) return "srt";
	if (name.endsWith(".vtt")) return "vtt";
	return undefined;
}

// "Import subtitles": the text of the file the person picked, read by the
// browser; the core parses it and judges the cues.
export function captionImportSubtitlesOperations(input: {
	trackId: string;
	text: string;
	format?: SubtitleFormat;
}): Operations {
	return [
		{
			op: "caption.import_subtitles",
			args: {
				track_id: input.trackId,
				text: input.text,
				...(input.format === undefined ? {} : { format: input.format }),
			},
		},
	];
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
// whole (replace: true, as the command merges by default), so the caller passes
// the stored style with the changed key.
export function captionStyleOperations(
	itemId: string,
	style: CaptionStyle | null,
): Operations {
	return [
		{
			op: "caption.set_style",
			args:
				style === null
					? { item_id: itemId, style: null }
					: { item_id: itemId, style: { ...style }, replace: true },
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

// A different word count: the core splits the text and spreads the words over
// the caption's duration (`caption.set_words` with `text`).
export function captionTextOperations(
	itemId: string,
	text: string,
): Operations {
	return [{ op: "caption.set_words", args: { item_id: itemId, text } }];
}

// The bulk edit: the words typed, one per whitespace-separated token, over the
// times of the stored words. Only the same count can keep the times, so a
// different count answers null and the caller sends the text instead.
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

// The colour properties of a clip that `keyframe.set` also animates (clips only).
export const KEYFRAME_COLOUR_PROPERTIES = [
	"brightness",
	"contrast",
	"saturation",
	"temperature",
	"tint",
	"hue",
] as const;
export type ColourKeyframeProperty =
	(typeof KEYFRAME_COLOUR_PROPERTIES)[number];

// The mask box and the crop sides `keyframe.set` also animates: the mask ones
// need the item to have a mask, the crop ones are for clips.
export const KEYFRAME_MASK_PROPERTIES = [
	"mask_x",
	"mask_y",
	"mask_width",
	"mask_height",
	"mask_feather",
] as const;
export const KEYFRAME_CROP_PROPERTIES = [
	"crop_left",
	"crop_top",
	"crop_right",
	"crop_bottom",
] as const;
export const KEYFRAME_FRAMING_PROPERTIES = [
	...KEYFRAME_MASK_PROPERTIES,
	...KEYFRAME_CROP_PROPERTIES,
] as const;
export type FramingKeyframeProperty =
	(typeof KEYFRAME_FRAMING_PROPERTIES)[number];
export type AnyKeyframeProperty =
	| KeyframeProperty
	| ColourKeyframeProperty
	| FramingKeyframeProperty;

// The stored value of a mask or crop property, undefined when the item has
// none of it.
export function framingStored(
	item: Pick<Item, "mask" | "crop">,
	property: FramingKeyframeProperty,
): number | undefined {
	if (property.startsWith("mask_")) {
		const key = property.slice(5) as "x" | "y" | "width" | "height" | "feather";
		return item.mask?.[key];
	}
	const side = property.slice(5) as "left" | "top" | "right" | "bottom";
	return item.crop?.[side] ?? 0;
}

// What the core uses for a colour property the item stores nothing for.
export const COLOUR_FALLBACK: Readonly<Record<ColourKeyframeProperty, number>> =
	{
		brightness: 0,
		contrast: 1,
		saturation: 1,
		temperature: 0,
		tint: 0,
		hue: 0,
	};

export interface StoredKeyframe {
	t: number;
	value: number;
	easing?: KeyframeEasing;
}

// The keyframe a list holds at a frame of the timeline, if any.
export function keyframeAt(
	keys: readonly StoredKeyframe[],
	itemStart: number,
	frame: number,
): StoredKeyframe | undefined {
	return keys.find((key) => itemStart + key.t === frame);
}

// What a colour slider of an animated property shows: the keyframe at the
// playhead, else the latest one before it, else the first. A keyframe of the
// document, never a value worked out between two (the plan does not report
// colour, so the page has nothing else to show).
export function shownColourKeyframe(
	keys: readonly StoredKeyframe[],
	itemStart: number,
	frame: number,
): StoredKeyframe | undefined {
	const exact = keyframeAt(keys, itemStart, frame);
	if (exact !== undefined) return exact;
	const before = keys.filter((key) => itemStart + key.t < frame);
	return before[before.length - 1] ?? keys[0];
}

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
	property: AnyKeyframeProperty;
	frame: number;
	value: number;
	easing?: KeyframeEasing;
}): Operations {
	return [
		{
			op: "keyframe.set",
			args: {
				item_id: input.itemId,
				property: input.property,
				at: frames(input.frame),
				value: input.value,
				...(input.easing === undefined ? {} : { easing: input.easing }),
			},
		},
	];
}

export function keyframeRemoveOperations(
	itemId: string,
	property: AnyKeyframeProperty,
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
	property?: AnyKeyframeProperty,
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
	"zoom",
	"whip",
	"dip",
	"wipe",
	"spin",
	"shrink",
	"grow",
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
// `color` goes with a dip and `direction` with a whip or a wipe only; the core
// refuses them on another kind, so the caller sends them for those kinds.
export function transitionOperations(input: {
	itemId: string;
	edge: Edge;
	kind: TransitionKind | null;
	duration?: string;
	color?: string;
	direction?: TransitionDirection;
}): Operations {
	return [
		{
			op: "item.set_transition",
			args: {
				item_id: input.itemId,
				edge: input.edge,
				kind: input.kind,
				...(input.duration === undefined ? {} : { duration: input.duration }),
				...(input.color === undefined ? {} : { color: input.color }),
				...(input.direction === undefined
					? {}
					: { direction: input.direction }),
			},
		},
	];
}

export const TRANSITION_DIRECTIONS = ["left", "right", "up", "down"] as const;
export type TransitionDirection = (typeof TRANSITION_DIRECTIONS)[number];

// The kinds that take a colour and a direction (SDD-002 C.18).
export const transitionTakesColor = (kind: TransitionKind): boolean =>
	kind === "dip";
export const transitionTakesDirection = (kind: TransitionKind): boolean =>
	kind === "whip" || kind === "wipe";

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
// Two neighbours of one track need another visual track for the arriving item:
// `freeTrack` is one the caller found without items in the way, or null and the
// same batch adds one first. The core judges whether the move fits.
export function crossfadeOperations(
	a: ItemRange,
	b: ItemRange,
	duration: string = CROSSFADE_DURATION,
	sameTrack?: { freeTrack: string | null; newTrackId: () => string },
): Operations {
	const [from, to] = a.start <= b.start ? [a, b] : [b, a];
	const args: {
		from_item_id: string;
		to_item_id: string;
		duration: string;
		track_id?: string;
	} = { from_item_id: from.id, to_item_id: to.id, duration };
	const ops: Operations = [];
	if (sameTrack !== undefined && from.track === to.track) {
		let track = sameTrack.freeTrack;
		if (track === null) {
			track = sameTrack.newTrackId();
			ops.push({ op: "track.add", args: { kind: "visual", id: track } });
		}
		args.track_id = track;
	}
	ops.push({ op: "item.crossfade", args });
	return ops;
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

// Ctrl+D: a copy of each item on its own track; the core places it at the first
// free position after the source, so a second Ctrl+D never overlaps. The copies
// go in start order.
export function duplicateOperations(items: readonly ItemRange[]): Operations {
	return [...items]
		.sort((a, b) => a.start - b.start)
		.map((item) => ({
			op: "item.duplicate",
			args: { item_id: item.id },
		}));
}

// Mod+V: the copied items at the playhead, keeping the distances between them
// (the first one lands on `at`). A copy is `item.duplicate` on the source
// track; a cut one is `item.move`, so the item keeps its id and its history.
// The core refuses a position another item holds (`E_OVERLAP`), and the batch
// is atomic. Moves go leader first, as `nudgeOperations` does.
export function pasteOperations(
	items: readonly ItemRange[],
	at: number,
	cut: boolean,
): Operations {
	if (items.length === 0) return [];
	const first = Math.min(...items.map((item) => item.start));
	const delta = at - first;
	const ordered = [...items].sort((a, b) =>
		cut && delta > 0 ? b.start - a.start : a.start - b.start,
	);
	return ordered.map((item) => ({
		op: cut ? "item.move" : "item.duplicate",
		args: { item_id: item.id, start: frames(item.start + delta) },
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

// Mod+Alt+Left and Mod+Alt+Right, the slide edit (VV-036): each item moves by
// `delta` frames and the core keeps its neighbours touching it. Same order as
// `nudgeOperations`.
export function slideOperations(
	items: readonly ItemRange[],
	delta: number,
): Operations {
	const ordered = [...items].sort((a, b) =>
		delta > 0 ? b.start - a.start : a.start - b.start,
	);
	return ordered.map((item) => ({
		op: "item.move",
		args: {
			item_id: item.id,
			start: frames(Math.max(0, item.start + delta)),
			slide: true,
		},
	}));
}

// Mod+Alt+Shift+Left and Right, the roll edit (VV-036): the cut after each item
// moves by `delta` frames, the core moves the edge of the next item with it.
export function rollOperations(
	items: readonly ItemRange[],
	delta: number,
): Operations {
	return items.map((item) => ({
		op: "item.trim",
		args: {
			item_id: item.id,
			edge: "end",
			to: frames(Math.max(0, item.start + item.duration + delta)),
			roll: true,
		},
	}));
}

// Alt+, and Alt+., the slip edit (VV-036): the part of the source an item
// shows moves by `delta` frames; the item itself stays where it is.
export function slipOperations(
	items: readonly { id: string; sourceIn: number }[],
	delta: number,
): Operations {
	return items.map((item) => ({
		op: "item.slip",
		args: {
			item_id: item.id,
			source_in: frames(Math.max(0, item.sourceIn + delta)),
		},
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
// (`details.next_free`, a frame number as received) on the operation that
// failed (`opIndex`); the others of the batch stay as they are.
export function withNextFree(
	ops: Operations,
	nextFree: number,
	opIndex: number,
): Operations {
	return ops.map((operation, index) =>
		index !== opIndex
			? operation
			: operation.op === "clip.add" ||
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
	magnet?: boolean;
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

// "Change aspect": the core re-targets the size and rescales text, shapes and
// fit none clips (SDD-002 C.60); the panel sends only the preset id.
export function reframeOperations(aspect: SizePresetId): Operations {
	return [{ op: "settings.reframe", args: { aspect } }];
}

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

export interface AudioChanges {
	volume?: number;
	fade_in?: string;
	fade_out?: string;
	audio_muted?: boolean;
	denoise?: number;
	voice?: VoicePreset;
}

export const VOICE_PRESETS = [
	"none",
	"clear",
	"warm",
	"deep",
	"bright",
	"radio",
] as const;
export type VoicePreset = (typeof VOICE_PRESETS)[number];

// Only the fields the person changed (SDD-001 section 3.5); the fades are lengths with
// a unit, as the time field normalized them.
export function audioOperations(
	itemId: string,
	changes: AudioChanges,
): Operations {
	return [{ op: "item.set_audio", args: { item_id: itemId, ...changes } }];
}

// Mod+. and Mod+,: the volume (a linear gain, 0 to 4) of each item moved by `db`
// decibels, to the 3 decimals the core keeps (SDD-002 A.10).
export function volumeStepOperations(
	items: readonly { id: string; volume: number }[],
	db: number,
): Operations {
	return items.flatMap(({ id, volume }) => {
		const next = Math.min(
			4,
			Math.max(0, Math.round(volume * 10 ** (db / 20) * 1000) / 1000),
		);
		return next === volume ? [] : audioOperations(id, { volume: next });
	});
}

export const FITS = ["cover", "contain", "none"] as const;
export type Fit = (typeof FITS)[number];

export type CropSide = "left" | "top" | "right" | "bottom";
export const CROP_SIDES: readonly CropSide[] = [
	"left",
	"top",
	"right",
	"bottom",
];

// A crop merges with the stored one side by side in the core (SDD-002 decision
// 23), so a change names only the side that changed.
export function framingOperations(
	itemId: string,
	changes: { fit?: Fit; crop?: Partial<Record<CropSide, number>> },
): Operations {
	return [{ op: "clip.set_framing", args: { item_id: itemId, ...changes } }];
}

// The speed goes as the text the person typed with the `x` suffix; the core
// parses it, bounds it to 1/4 and 4 and limits it to three decimals.
export function speedOperations(
	itemId: string,
	speed: number,
	ripple: boolean,
): Operations {
	return [
		{
			op: "clip.set_speed",
			args: {
				item_id: itemId,
				speed: `${speed}x`,
				...(ripple ? { ripple: "track" } : {}),
			},
		},
	];
}

// A point of the speed curve as the person edits it: `t` is a fraction of the
// item (0 to 1) and `speed` the multiplier. The core checks the 2 to 8 points,
// the bounds and the order (SDD-002 C.25).
export interface CurvePoint {
	t: number;
	speed: number;
}

const round3 = (value: number): number => Math.round(value * 1000) / 1000;

// "Add point": one in the middle of the widest gap, on the line between its
// two neighbours, so the curve keeps its shape.
export function insertCurvePoint(points: readonly CurvePoint[]): CurvePoint[] {
	let at = 0;
	for (let i = 1; i + 1 < points.length; i++) {
		const gap = (points[i + 1]?.t ?? 0) - (points[i]?.t ?? 0);
		if (gap > (points[at + 1]?.t ?? 0) - (points[at]?.t ?? 0)) at = i;
	}
	const a = points[at];
	const b = points[at + 1];
	if (a === undefined || b === undefined) return [...points];
	const inserted = {
		t: round3((a.t + b.t) / 2),
		speed: round3((a.speed + b.speed) / 2),
	};
	return [...points.slice(0, at + 1), inserted, ...points.slice(at + 1)];
}

export function speedCurveOperations(
	itemId: string,
	points: readonly CurvePoint[],
	ripple: boolean,
): Operations {
	return [
		{
			op: "clip.set_speed",
			args: {
				item_id: itemId,
				curve: points.map((point) => ({
					t: round3(point.t),
					speed: `${round3(point.speed)}x`,
				})),
				...(ripple ? { ripple: "track" } : {}),
			},
		},
	];
}

// "Remove curve": the curve goes and the speed is its stored mean, as the exact
// fraction the read model holds, so the duration does not change.
export function constantSpeedOperations(
	itemId: string,
	speed: { num: number; den: number },
): Operations {
	return [
		{
			op: "clip.set_speed",
			args: { item_id: itemId, speed: { num: speed.num, den: speed.den } },
		},
	];
}

// `reverse` alone changes nothing else: the duration, speed and curve stay.
export function reverseOperations(
	itemId: string,
	reverse: boolean,
): Operations {
	return [{ op: "clip.set_speed", args: { item_id: itemId, reverse } }];
}

export const EFFECT_KINDS = [
	"motion_blur",
	"glow",
	"vignette",
	"grain",
	"rgb_split",
	"shake",
] as const;
export type EffectKind = (typeof EFFECT_KINDS)[number];

// The parameters of each kind with the range and the default of SDD-002 C.68,
// in the order of SDD-004 section 3.13. The inputs show these bounds; the core
// is the one that checks them.
export interface EffectParam {
	name:
		| "amount"
		| "angle"
		| "radius"
		| "threshold"
		| "softness"
		| "size"
		| "speed"
		| "seed";
	min: number;
	max: number;
	step: number;
	default: number;
}

export const EFFECT_PARAMS: Readonly<
	Record<EffectKind, readonly EffectParam[]>
> = {
	motion_blur: [
		{ name: "amount", min: 0, max: 1, step: 0.05, default: 0.5 },
		{ name: "angle", min: -180, max: 180, step: 1, default: 0 },
	],
	glow: [
		{ name: "amount", min: 0, max: 1, step: 0.05, default: 0.5 },
		{ name: "radius", min: 1, max: 100, step: 1, default: 12 },
		{ name: "threshold", min: 0, max: 1, step: 0.05, default: 0.6 },
	],
	vignette: [
		{ name: "amount", min: 0, max: 1, step: 0.05, default: 0.5 },
		{ name: "softness", min: 0, max: 1, step: 0.05, default: 0.5 },
	],
	grain: [
		{ name: "amount", min: 0, max: 1, step: 0.05, default: 0.3 },
		{ name: "size", min: 1, max: 8, step: 1, default: 1 },
		{ name: "seed", min: 0, max: 65535, step: 1, default: 0 },
	],
	rgb_split: [
		{ name: "amount", min: 0, max: 1, step: 0.05, default: 0.3 },
		{ name: "angle", min: -180, max: 180, step: 1, default: 0 },
	],
	shake: [
		{ name: "amount", min: 0, max: 1, step: 0.05, default: 0.3 },
		{ name: "speed", min: 1, max: 30, step: 1, default: 8 },
		{ name: "seed", min: 0, max: 65535, step: 1, default: 0 },
	],
};

export type EffectValue = { kind: EffectKind } & Partial<
	Record<EffectParam["name"], number>
>;

// The whole list is the argument (SDD-002 C.68); `[]` removes every effect.
// A new effect of `kind` takes the defaults of its kind.
export function effectsOperations(
	itemId: string,
	effects: readonly EffectValue[],
): Operations {
	return [
		{
			op: "clip.set_effects",
			args: { item_id: itemId, effects: [...effects] },
		},
	];
}

export function defaultEffect(kind: EffectKind): EffectValue {
	const effect: EffectValue = { kind };
	for (const param of EFFECT_PARAMS[kind]) effect[param.name] = param.default;
	return effect;
}

// `item.link`: the group id is the core's to generate.
export function linkOperations(itemIds: readonly string[]): Operations {
	return [{ op: "item.link", args: { item_ids: [...itemIds] } }];
}

// The groups the menu and the shortcut paste with `item.paste_attributes`; a group
// the target type does not have is skipped by the core and listed in
// `effective.skipped`. `flip` is left out on purpose: it is the orientation of the
// picture, not part of its look, and pasting it would reset a mirrored target
// whenever the source is not mirrored (VV-184). The command still accepts `flip`.
export const PASTE_GROUPS = [
	"color",
	"filter",
	"blur",
	"framing",
	"transform",
	"blend",
	"mask",
	"chroma_key",
	"effects",
	"audio",
] as const;

export function pasteAttributesOperations(
	fromItemId: string,
	itemIds: readonly string[],
): Operations {
	return itemIds.length === 0
		? []
		: [
				{
					op: "item.paste_attributes",
					args: {
						from_item_id: fromItemId,
						item_ids: [...itemIds],
						attributes: [...PASTE_GROUPS],
					},
				},
			];
}

export function unlinkOperations(itemIds: readonly string[]): Operations {
	return [{ op: "item.unlink", args: { item_ids: [...itemIds] } }];
}

// The audio of a video clip goes to an audio track as a linked item, and the
// clip is muted. The track is one the caller found, or a new one added first.
export function extractAudioOperations(input: {
	itemId: string;
	audioTrack: string | null;
	newTrackId: () => string;
}): Operations {
	const ops: Operations = [];
	let track = input.audioTrack;
	if (track === null) {
		track = input.newTrackId();
		ops.push({ op: "track.add", args: { kind: "audio", id: track } });
	}
	ops.push({
		op: "item.extract_audio",
		args: { item_id: input.itemId, track_id: track },
	});
	return ops;
}

export interface ColorChanges {
	brightness?: number;
	contrast?: number;
	saturation?: number;
	temperature?: number;
	tint?: number;
	hue?: number;
	// The id of a lut asset, and how much of it is applied (0 removes it).
	lut_asset?: string;
	lut_intensity?: number;
	// A band, a wheel or a curve that is given replaces the stored one; a neutral
	// one removes it (SDD-001 section 3.5).
	hsl?: HslBandValue[];
	wheels?: { [name in WheelName]?: WheelValue };
	curves?: { [channel in CurveChannel]?: CurvePointValue[] };
}

export const HSL_RANGES = [
	"red",
	"orange",
	"yellow",
	"green",
	"cyan",
	"blue",
	"purple",
	"magenta",
] as const;
export type HslRange = (typeof HSL_RANGES)[number];

export type HslBandValue = {
	range: HslRange;
	hue: number;
	saturation: number;
	lightness: number;
};

export const WHEEL_NAMES = ["shadows", "midtones", "highlights"] as const;
export type WheelName = (typeof WHEEL_NAMES)[number];

export type WheelValue = {
	hue: number;
	amount: number;
	luma: number;
};

export const CURVE_CHANNELS = ["master", "red", "green", "blue"] as const;
export type CurveChannel = (typeof CURVE_CHANNELS)[number];

export type CurvePointValue = {
	x: number;
	y: number;
};

// "Reset" of the three advanced groups: the neutral value of what the item
// stores, which the core reads as removing it (an empty list for a curve).
export function colorResetChanges(
	group: "hsl" | "wheels" | "curves",
	stored: {
		hsl: readonly HslRange[];
		wheels: readonly WheelName[];
		curves: readonly CurveChannel[];
	},
): ColorChanges {
	if (group === "hsl") {
		return {
			hsl: stored.hsl.map((range) => ({
				range,
				hue: 0,
				saturation: 0,
				lightness: 0,
			})),
		};
	}
	if (group === "wheels") {
		return {
			wheels: Object.fromEntries(
				stored.wheels.map((name) => [name, { hue: 0, amount: 0, luma: 0 }]),
			),
		};
	}
	return {
		curves: Object.fromEntries(stored.curves.map((name) => [name, []])),
	};
}

export function colorOperations(
	itemId: string,
	changes: ColorChanges,
): Operations {
	return [{ op: "clip.set_color", args: { item_id: itemId, ...changes } }];
}

export interface SilenceOptions {
	minSilence: string;
	padding: string;
	ripple: boolean;
}

// The ranges to remove come from the service (it measures the asset); the
// request carries only the person's choices (SDD-002 C.27).
export function silenceOperations(
	itemId: string,
	options: SilenceOptions,
): Operations {
	return [
		{
			op: "clip.remove_silences",
			args: {
				item_id: itemId,
				min_silence: options.minSilence,
				padding: options.padding,
				ripple: options.ripple ? "track" : "none",
			},
		},
	];
}

export const SHAPES = ["rect", "rounded_rect", "ellipse"] as const;
export type ShapeKind = (typeof SHAPES)[number];

// The size of a shape added from the toolbar, in pixels before `scale`.
export const DEFAULT_SHAPE_SIZE = { width: 400, height: 240 } as const;

// Toolbar "Add shape": a shape of 3 s at the playhead; the core picks the
// track as for text (SDD-002 C.6, C.31).
export function shapeAddOperations(input: {
	shape: ShapeKind;
	playhead: number;
}): Operations {
	return [
		{
			op: "shape.add",
			args: {
				start: frames(input.playhead),
				duration: "3s",
				shape: input.shape,
				...DEFAULT_SHAPE_SIZE,
			},
		},
	];
}

export interface ShapeChanges {
	shape?: ShapeKind;
	width?: number;
	height?: number;
	radius?: number;
	fill?: string;
	stroke_color?: string;
	stroke_width?: number;
}

// One field of the shape section: only the changed key is sent.
export function shapeSetOperations(
	itemId: string,
	changes: ShapeChanges,
): Operations {
	return [{ op: "shape.set", args: { item_id: itemId, ...changes } }];
}

export interface SlotMark {
	name: string;
	description: string;
	required: boolean;
	// Media slots only (SDD-002 C.45).
	durationPolicy?: "fit" | "stretch_timeline";
}

// "Template slot" on. Re-marking with the same name updates the slot, so every
// field of the section is sent each time: the core applies the defaults of the
// ones that are left out.
export function slotMarkOperations(itemId: string, mark: SlotMark): Operations {
	return [
		{
			op: "slot.mark",
			args: {
				item_id: itemId,
				name: mark.name,
				description: mark.description,
				required: mark.required,
				...(mark.durationPolicy === undefined
					? {}
					: { duration_policy: mark.durationPolicy }),
			},
		},
	];
}

// The name a slot gets when the toggle is switched on: the first `slot_N`
// that no item of the project uses, so the mark is sent at once.
export function nextSlotName(taken: Iterable<string | undefined>): string {
	const used = new Set(taken);
	let n = 1;
	while (used.has(`slot_${n}`)) n++;
	return `slot_${n}`;
}

// "Template slot" off.
export function slotUnmarkOperations(itemId: string): Operations {
	return [{ op: "slot.unmark", args: { item_id: itemId } }];
}

// "Fill" of a row of the slots panel: a media slot takes an asset, a text slot
// its text (a caption slot takes words, filled from the CLI, MCP or API).
export type SlotFill = { asset_id: string } | { text: string };

export function slotFillOperations(name: string, value: SlotFill): Operations {
	return [{ op: "slot.fill", args: { name, ...value } }];
}

export interface TemplateInfoChanges {
	description?: string;
	// A name to a colour, or null to remove the name.
	colors?: Record<string, string | null>;
}

export function templateInfoOperations(
	changes: TemplateInfoChanges,
): Operations {
	return [{ op: "template.set_info", args: { ...changes } }];
}

export const FILTERS = [
	"none",
	"grayscale",
	"sepia",
	"invert",
	"warm",
	"cool",
] as const;
export type Filter = (typeof FILTERS)[number];

export interface FilterChanges {
	filter?: Filter;
	amount?: number;
}

// Only the field the person changed (SDD-002 C.49); the core clears a filter
// that returns to its default.
export function filterOperations(
	itemId: string,
	changes: FilterChanges,
): Operations {
	return [{ op: "clip.set_filter", args: { item_id: itemId, ...changes } }];
}

export interface ChromaKeyChanges {
	similarity?: number;
	smoothness?: number;
	spill?: number;
}

// A colour turns the key on (or changes its colour) and null removes it; the
// settings not sent keep their value (`clip.set_chroma_key`).
export function chromaKeyOperations(
	itemId: string,
	color: string | null,
	changes: ChromaKeyChanges = {},
): Operations {
	return [
		{
			op: "clip.set_chroma_key",
			args: { item_id: itemId, color, ...changes },
		},
	];
}

export type ShadowArgs = {
	color?: string;
	opacity?: number;
	dx?: number;
	dy?: number;
	blur?: number;
};

// A shadow object turns the shadow on or changes it (settings not sent keep
// their value); null removes it (`clip.set_shadow`).
export function shadowOperations(
	itemId: string,
	shadow: ShadowArgs | null,
): Operations {
	return [{ op: "clip.set_shadow", args: { item_id: itemId, shadow } }];
}

export type OutlineArgs = {
	color?: string;
	width?: number;
};

// As the shadow: an object sets, null removes (`clip.set_outline`).
export function outlineOperations(
	itemId: string,
	outline: OutlineArgs | null,
): Operations {
	return [{ op: "clip.set_outline", args: { item_id: itemId, outline } }];
}

export function markerAddOperations(frame: number): Operations {
	return [{ op: "marker.add", args: { at: frames(frame) } }];
}

export function markerRemoveOperations(
	frame: number,
	itemId?: string,
): Operations {
	return [
		{
			op: "marker.remove",
			args: {
				at: frames(frame),
				...(itemId === undefined ? {} : { item_id: itemId }),
			},
		},
	];
}

// A marker changed: its label (`""` removes it) or its position (`to`, a time
// with a unit). Without `itemId` it is a marker of the timeline.
export function markerSetOperations(
	frame: number,
	changes: { label?: string; to?: string },
	itemId?: string,
): Operations {
	return [
		{
			op: "marker.set",
			args: {
				at: frames(frame),
				...(itemId === undefined ? {} : { item_id: itemId }),
				...changes,
			},
		},
	];
}

export const BLEND_MODES = [
	"normal",
	"multiply",
	"screen",
	"overlay",
	"add",
] as const;
export type BlendMode = (typeof BLEND_MODES)[number];

export function blendOperations(itemId: string, mode: BlendMode): Operations {
	return [{ op: "clip.set_blend", args: { item_id: itemId, mode } }];
}

export type MaskShapeName = "rect" | "ellipse";

export type MaskArgs = {
	shape: MaskShapeName;
	x: number;
	y: number;
	width: number;
	height: number;
	feather: number;
	invert: boolean;
};

// A whole mask replaces the current one; null removes it (`clip.set_mask`).
export function maskOperations(
	itemId: string,
	mask: MaskArgs | null,
): Operations {
	return [{ op: "clip.set_mask", args: { item_id: itemId, mask } }];
}

// A side not named keeps its value in the core (SDD-002 C.63).
export function flipOperations(
	itemId: string,
	changes: { horizontal?: boolean; vertical?: boolean },
): Operations {
	return [{ op: "clip.set_flip", args: { item_id: itemId, ...changes } }];
}

export function enabledOperations(
	itemId: string,
	enabled: boolean,
): Operations {
	return [{ op: "item.set_enabled", args: { item_id: itemId, enabled } }];
}

export function blurOperations(itemId: string, radius: number): Operations {
	return [{ op: "clip.set_blur", args: { item_id: itemId, radius } }];
}

// What the core uses when a request names no gain, attack or release
// (SDD-002 C.51); the panels start there and the choices go with the button.
export const DEFAULT_GAIN_DB = -12;
export const DEFAULT_ATTACK = "0.2s";
export const DEFAULT_RELEASE = "0.5s";

export interface DuckOptions {
	speechItemIds: readonly string[];
	gainDb: number;
	attack: string;
	release: string;
}

// The speech stretches come from the service (it analyses the speech items and
// fills `resolved`); the request carries only the person's choices.
export function duckOperations(
	itemId: string,
	options: DuckOptions,
): Operations {
	return [
		{
			op: "audio.duck",
			args: {
				item_id: itemId,
				speech_item_ids: [...options.speechItemIds],
				gain_db: options.gainDb,
				attack: options.attack,
				release: options.release,
			},
		},
	];
}

export interface BeatOptions {
	every: number;
	label: string;
	replace: boolean;
}

// The beats come from the beat grid of the asset, filled by the service; an
// empty label is left out.
export function markBeatsOperations(
	itemId: string,
	options: BeatOptions,
): Operations {
	return [
		{
			op: "audio.mark_beats",
			args: {
				item_id: itemId,
				every: options.every,
				replace: options.replace,
				...(options.label === "" ? {} : { label: options.label }),
			},
		},
	];
}

export type FontFileInput = {
	weight: number;
	hash: string;
};

export function fontAddOperations(
	family: string,
	files: readonly FontFileInput[],
): Operations {
	return [{ op: "font.add", args: { family, files: [...files] } }];
}

export function fontRemoveOperations(family: string): Operations {
	return [{ op: "font.remove", args: { family } }];
}
