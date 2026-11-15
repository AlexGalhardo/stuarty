import { describe, expect, test } from "vitest";

import {
	addClipOperations,
	boundaryWords,
	bulkEditWords,
	captionFromTranscriptOperations,
	captionOperations,
	captionRetimeOperations,
	clampBoundary,
	containsInclusive,
	crossfadeOperations,
	keyframeClearOperations,
	keyframeFallbackOperations,
	keyframeFrame,
	keyframeRemoveOperations,
	keyframeSetOperations,
	moveItems,
	moveOperations,
	nudgeOperations,
	presetOperations,
	splitOperations,
	textAddOperations,
	textSetOperations,
	textStyleOperations,
	toggleBreak,
	transformOperations,
	transitionOperations,
	trimEdgeOperations,
	trimOperations,
	withNextFree,
} from "./builders";

// Rule T8: every time a builder emits is a string with a unit, never a bare number.
function times(ops: ReturnType<typeof moveItems>): unknown[] {
	return ops.flatMap((op) =>
		Object.entries(op.args)
			.filter(([key]) => ["start", "at", "to", "duration"].includes(key))
			.map(([, value]) => value),
	);
}

const range = (id: string, start: number) => ({ id, start, duration: 30 });

describe("builders emit times as strings with a unit (T8)", () => {
	test("moveItems", () => {
		const ops = moveItems([
			{ id: "a", from: 0, to: 162 },
			{ id: "b", from: 30, to: 192, trackId: "trk_2" },
		]);
		for (const value of times(ops)) expect(value).toMatch(/^\d+f$/);
		expect(ops.map((o) => o.args.item_id)).toEqual(["b", "a"]);
	});

	test("the other builders", () => {
		const all = [
			...nudgeOperations([range("a", 5)], -9),
			...moveOperations("a", "12f"),
			...splitOperations([range("a", 0)], 10),
			...trimOperations([range("a", 0)], "end", 10),
			...addClipOperations({
				assetId: "ast_1",
				playhead: 7,
				firstVisualTrack: "trk_1",
				newTrackId: () => "trk_n",
			}),
			...captionOperations({
				text: "hi",
				duration: "3s",
				playhead: 4,
				firstVisualTrack: "trk_1",
				newTrackId: () => "trk_n",
			}),
			...withNextFree(moveItems([{ id: "a", from: 0, to: 5 }]), 40),
		];
		for (const value of times(all)) expect(typeof value).toBe("string");
		for (const value of times(all)) expect(value).toMatch(/^\d+(\.\d+)?[fs]$/);
	});
});

describe("moveItems", () => {
	test("emits only what changed", () => {
		expect(
			moveItems([
				{ id: "a", from: 10, to: 10 },
				{ id: "b", from: 10, to: 20 },
				{ id: "c", from: 10, to: 10, trackId: "trk_2" },
				{ id: "d", from: 10, to: 3, trackId: "trk_2" },
			]),
		).toEqual([
			{ op: "item.move", args: { item_id: "b", start: "20f" } },
			{ op: "item.move", args: { item_id: "c", track_id: "trk_2" } },
			{
				op: "item.move",
				args: { item_id: "d", track_id: "trk_2", start: "3f" },
			},
		]);
	});

	test("nothing changed sends nothing", () => {
		expect(moveItems([{ id: "a", from: 4, to: 4 }])).toEqual([]);
	});

	test("moving right the last item goes first, moving left the first", () => {
		const right = moveItems([
			{ id: "a", from: 0, to: 5 },
			{ id: "b", from: 30, to: 35 },
		]);
		expect(right.map((o) => o.args.item_id)).toEqual(["b", "a"]);
		const left = moveItems([
			{ id: "b", from: 30, to: 25 },
			{ id: "a", from: 10, to: 5 },
		]);
		expect(left.map((o) => o.args.item_id)).toEqual(["a", "b"]);
	});
});

describe("trim", () => {
	test("a released handle is one item.trim with a time unit", () => {
		expect(trimEdgeOperations("a", "end", 88)).toEqual([
			{ op: "item.trim", args: { item_id: "a", edge: "end", to: "88f" } },
		]);
	});

	test("the next free place of the core replaces `to`, the edge stays", () => {
		expect(withNextFree(trimEdgeOperations("a", "start", 4), 12)).toEqual([
			{ op: "item.trim", args: { item_id: "a", edge: "start", to: "12f" } },
		]);
	});
});

describe("transformOperations", () => {
	test("static properties go in one item.set_transform", () => {
		expect(
			transformOperations([{ id: "itm_a", animated: [] }], { x: 10, y: 20 }, 5),
		).toEqual([
			{ op: "item.set_transform", args: { item_id: "itm_a", x: 10, y: 20 } },
		]);
	});

	test("an animated property goes in a keyframe.set at the playhead (gating by key only)", () => {
		expect(
			transformOperations(
				[{ id: "itm_a", animated: ["scale"] }],
				{ scale: 1.5 },
				12,
			),
		).toEqual([
			{
				op: "keyframe.set",
				args: { item_id: "itm_a", property: "scale", at: "12f", value: 1.5 },
			},
		]);
	});

	test("a change of an animated and a static property sends both", () => {
		const ops = transformOperations(
			[{ id: "itm_a", animated: ["x"] }],
			{ x: 1, y: 2 },
			3,
		);
		expect(ops.map((o) => o.op)).toEqual([
			"item.set_transform",
			"keyframe.set",
		]);
		expect(ops[0]?.args).toEqual({ item_id: "itm_a", y: 2 });
	});

	test("a multi-selection gets one batch, each item by its own keys", () => {
		const ops = transformOperations(
			[
				{ id: "itm_a", animated: [] },
				{ id: "itm_b", animated: ["opacity"] },
			],
			{ opacity: 0.5 },
			0,
		);
		expect(ops.map((o) => [o.op, o.args.item_id])).toEqual([
			["item.set_transform", "itm_a"],
			["keyframe.set", "itm_b"],
		]);
	});

	test("the fallback of E_INVALID_STATE resends every field as keyframe.set", () => {
		const ops = keyframeFallbackOperations(
			transformOperations(
				[{ id: "itm_a", animated: [] }],
				{ x: 1, rotation: 30 },
				7,
			),
			9,
		);
		expect(ops).toEqual([
			{
				op: "keyframe.set",
				args: { item_id: "itm_a", property: "x", at: "9f", value: 1 },
			},
			{
				op: "keyframe.set",
				args: { item_id: "itm_a", property: "rotation", at: "9f", value: 30 },
			},
		]);
	});
});

describe("text builders", () => {
	test("Add text is 3 s at the playhead on the first visual track", () => {
		expect(
			textAddOperations({
				text: "Your text",
				playhead: 12,
				firstVisualTrack: "trk_1",
				newTrackId: () => "trk_n",
			}),
		).toEqual([
			{
				op: "text.add",
				args: {
					track_id: "trk_1",
					start: "12f",
					duration: "3s",
					text: "Your text",
				},
			},
		]);
	});

	test("Add text creates the visual track in the same batch when there is none", () => {
		const ops = textAddOperations({
			text: "t",
			playhead: 0,
			firstVisualTrack: null,
			newTrackId: () => "trk_n",
		});
		expect(ops.map((o) => o.op)).toEqual(["track.add", "text.add"]);
		expect(ops[1]?.args.track_id).toBe("trk_n");
	});

	test("the content and one style key are separate partial requests", () => {
		expect(textSetOperations("itm_a", "x")).toEqual([
			{ op: "text.set", args: { item_id: "itm_a", text: "x" } },
		]);
		expect(textStyleOperations("itm_a", { font_family: "Inter" })).toEqual([
			{
				op: "text.set",
				args: { item_id: "itm_a", style: { font_family: "Inter" } },
			},
		]);
	});
});

describe("caption word builders", () => {
	const words = [
		{ text: "a", start: 0, duration: 10 },
		{ text: "b", start: 10, duration: 10 },
		{ text: "c", start: 25, duration: 5 },
	];

	test("a boundary between contiguous words moves both and copies the rest", () => {
		expect(boundaryWords(words, 1, 14)).toEqual([
			{ text: "a", start: "0f", duration: "14f" },
			{ text: "b", start: "14f", duration: "6f" },
			{ text: "c", start: "25f", duration: "5f" },
		]);
	});

	test("with a gap before the word, the word before only gives way", () => {
		expect(boundaryWords(words, 2, 27)[1]).toEqual({
			text: "b",
			start: "10f",
			duration: "10f",
		});
		expect(boundaryWords(words, 2, 27)[2]).toEqual({
			text: "c",
			start: "27f",
			duration: "3f",
		});
		expect(boundaryWords(words, 2, 15)[1]?.duration).toBe("5f");
	});

	test("the boundary stays a frame inside the two words", () => {
		expect(clampBoundary(words, 1, -5)).toBe(1);
		expect(clampBoundary(words, 1, 99)).toBe(19);
	});

	test("breaks stay strictly increasing and toggle off", () => {
		expect(toggleBreak([4], 2)).toEqual([2, 4]);
		expect(toggleBreak([2, 4], 2)).toEqual([4]);
	});

	test("the bulk edit needs the same word count", () => {
		expect(bulkEditWords(words, "x  y\nz")?.map((w) => w.text)).toEqual([
			"x",
			"y",
			"z",
		]);
		expect(bulkEditWords(words, "x y")).toBeNull();
	});
});

describe("auto caption builders", () => {
	const input = {
		clipId: "itm_clip",
		newTrackId: () => "trk_new",
		trackName: "Captions",
		maxWords: 5,
		maxChars: 28,
		maxGap: "0.6s",
		replace: true,
	};

	test("a chosen track gets only caption.from_transcript", () => {
		expect(
			captionFromTranscriptOperations({ ...input, trackId: "trk_a" }),
		).toEqual([
			{
				op: "caption.from_transcript",
				args: {
					clip_item_id: "itm_clip",
					track_id: "trk_a",
					max_words: 5,
					max_chars: 28,
					max_gap: "0.6s",
					replace: true,
				},
			},
		]);
	});

	test("no track adds a Captions track in the same batch", () => {
		const ops = captionFromTranscriptOperations({ ...input, trackId: null });
		expect(ops.map((o) => o.op)).toEqual([
			"track.add",
			"caption.from_transcript",
		]);
		expect(ops[0]?.args).toEqual({
			kind: "visual",
			id: "trk_new",
			name: "Captions",
		});
		expect(ops[1]?.args.track_id).toBe("trk_new");
	});

	test("re-sync names the clip only", () => {
		expect(captionRetimeOperations("itm_clip")).toEqual([
			{ op: "caption.retime_from_clip", args: { clip_item_id: "itm_clip" } },
		]);
	});
});

describe("keyframe builders", () => {
	test("set names the position on the timeline and the easing as picked", () => {
		expect(
			keyframeSetOperations({
				itemId: "itm_a",
				property: "scale",
				frame: 45,
				value: 1.5,
				easing: "ease_in_out",
			}),
		).toEqual([
			{
				op: "keyframe.set",
				args: {
					item_id: "itm_a",
					property: "scale",
					at: "45f",
					value: 1.5,
					easing: "ease_in_out",
				},
			},
		]);
	});

	test("a stored keyframe is addressed by its position on the timeline", () => {
		expect(keyframeFrame(100, 30)).toBe(130);
		expect(keyframeRemoveOperations("itm_a", "x", 130)).toEqual([
			{
				op: "keyframe.remove",
				args: { item_id: "itm_a", property: "x", at: "130f" },
			},
		]);
	});

	test("clear names one property or none", () => {
		expect(keyframeClearOperations("itm_a", "opacity")[0]?.args).toEqual({
			item_id: "itm_a",
			property: "opacity",
		});
		expect(keyframeClearOperations("itm_a")[0]?.args).toEqual({
			item_id: "itm_a",
		});
	});

	test("the playhead may touch both ends of the item", () => {
		const item = { id: "itm_a", start: 10, duration: 30 };
		expect(containsInclusive(item, 10)).toBe(true);
		expect(containsInclusive(item, 40)).toBe(true);
		expect(containsInclusive(item, 9)).toBe(false);
		expect(containsInclusive(item, 41)).toBe(false);
	});
});

describe("transition, preset and crossfade builders", () => {
	test("a kind with no duration leaves the length to the core", () => {
		expect(
			transitionOperations({ itemId: "itm_a", edge: "in", kind: "fade" }),
		).toEqual([
			{
				op: "item.set_transition",
				args: { item_id: "itm_a", edge: "in", kind: "fade" },
			},
		]);
	});

	test("none sends a null kind and a duration keeps its unit", () => {
		expect(
			transitionOperations({ itemId: "itm_a", edge: "out", kind: null })[0]
				?.args.kind,
		).toBeNull();
		expect(
			transitionOperations({
				itemId: "itm_a",
				edge: "out",
				kind: "slide_up",
				duration: "0.3s",
			})[0]?.args.duration,
		).toBe("0.3s");
	});

	test("a preset names the item, the edge and the preset", () => {
		expect(
			presetOperations({
				itemId: "itm_a",
				edge: "in",
				preset: "zoom_in",
				duration: "0.4s",
			}),
		).toEqual([
			{
				op: "text.apply_preset",
				args: {
					item_id: "itm_a",
					edge: "in",
					preset: "zoom_in",
					duration: "0.4s",
				},
			},
		]);
	});

	test("the crossfade leaves from the item whose stored start is smaller", () => {
		const first = { id: "itm_a", start: 0, duration: 30 };
		const second = { id: "itm_b", start: 30, duration: 30 };
		for (const pair of [
			[first, second],
			[second, first],
		] as const) {
			expect(crossfadeOperations(pair[0], pair[1])).toEqual([
				{
					op: "item.crossfade",
					args: {
						from_item_id: "itm_a",
						to_item_id: "itm_b",
						duration: "0.5s",
					},
				},
			]);
		}
	});
});
