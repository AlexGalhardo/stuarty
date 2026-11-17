import { describe, expect, test } from "vitest";

import {
	addClipOperations,
	audioOperations,
	blurOperations,
	boundaryWords,
	bulkEditWords,
	captionFromTranscriptOperations,
	captionOperations,
	captionRetimeOperations,
	captionTextOperations,
	clampBoundary,
	colorOperations,
	containsInclusive,
	crossfadeOperations,
	duckOperations,
	enabledOperations,
	filterOperations,
	flipOperations,
	fontAddOperations,
	fontRemoveOperations,
	framingOperations,
	keyframeClearOperations,
	keyframeFallbackOperations,
	keyframeFrame,
	keyframeRemoveOperations,
	keyframeSetOperations,
	markBeatsOperations,
	markerRemoveOperations,
	markerSetOperations,
	moveItems,
	moveOperations,
	nextSlotName,
	nudgeOperations,
	pasteOperations,
	presetOperations,
	projectRenameOperations,
	reframeOperations,
	rollOperations,
	settingsFpsOperations,
	settingsSetOperations,
	shapeAddOperations,
	shapeSetOperations,
	silenceOperations,
	slideOperations,
	slipOperations,
	slotFillOperations,
	slotMarkOperations,
	slotUnmarkOperations,
	speedOperations,
	splitOperations,
	templateInfoOperations,
	textAddOperations,
	textSetOperations,
	textStyleOperations,
	toggleBreak,
	trackAddOperations,
	trackCaptionStyleOperations,
	trackMoveOperations,
	trackRemoveOperations,
	trackUpdateOperations,
	transformOperations,
	transitionOperations,
	trimEdgeOperations,
	trimOperations,
	volumeStepOperations,
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

describe("marker builders", () => {
	test("a marker is found by its frame, on the timeline or on an item", () => {
		expect(markerSetOperations(30, { label: "Hook" })).toEqual([
			{ op: "marker.set", args: { at: "30f", label: "Hook" } },
		]);
		expect(markerSetOperations(30, { to: "2s" }, "itm_a")).toEqual([
			{ op: "marker.set", args: { at: "30f", item_id: "itm_a", to: "2s" } },
		]);
		expect(markerRemoveOperations(30, "itm_a")).toEqual([
			{ op: "marker.remove", args: { at: "30f", item_id: "itm_a" } },
		]);
		expect(markerRemoveOperations(30)).toEqual([
			{ op: "marker.remove", args: { at: "30f" } },
		]);
	});
});

describe("builders emit times as strings with a unit (T8)", () => {
	test("moveItems", () => {
		const ops = moveItems([
			{ id: "a", from: 0, to: 162 },
			{ id: "b", from: 30, to: 192, trackId: "trk_2" },
		]);
		for (const value of times(ops)) expect(value).toMatch(/^\d+f$/);
		expect(ops.map((o) => o.args.item_id)).toEqual(["b", "a"]);
	});

	test("pasteOperations puts the first item on the playhead and keeps the gaps", () => {
		const items = [range("b", 40), range("a", 10)];
		expect(pasteOperations(items, 100, false)).toEqual([
			{ op: "item.duplicate", args: { item_id: "a", start: "100f" } },
			{ op: "item.duplicate", args: { item_id: "b", start: "130f" } },
		]);
		// A cut one moves; going right the leader goes first, going left the last.
		expect(
			pasteOperations(items, 100, true).map((o) => o.args.item_id),
		).toEqual(["b", "a"]);
		expect(pasteOperations(items, 0, true)).toEqual([
			{ op: "item.move", args: { item_id: "a", start: "0f" } },
			{ op: "item.move", args: { item_id: "b", start: "30f" } },
		]);
		expect(pasteOperations([], 5, false)).toEqual([]);
	});

	test("the other builders", () => {
		const all = [
			...nudgeOperations([range("a", 5)], -9),
			...moveOperations("a", "12f"),
			...splitOperations([range("a", 0)], 10),
			...trimOperations([range("a", 0)], "end", 10),
			...addClipOperations({
				assetId: "ast_1",
				firstVisualTrack: "trk_1",
				newTrackId: () => "trk_n",
			}),
			...captionOperations({
				text: "hi",
				duration: "3s",
				playhead: 4,
			}),
			...withNextFree(moveItems([{ id: "a", from: 0, to: 5 }]), 40, 0),
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
		expect(withNextFree(trimEdgeOperations("a", "start", 4), 12, 0)).toEqual([
			{ op: "item.trim", args: { item_id: "a", edge: "start", to: "12f" } },
		]);
	});
});

describe("withNextFree", () => {
	test("only the failed operation of the batch is moved", () => {
		const ops = moveItems([
			{ id: "a", from: 0, to: 5 },
			{ id: "b", from: 0, to: 6 },
			{ id: "c", from: 0, to: 7 },
		]);
		const out = withNextFree(ops, 40, 1);
		expect(out[0]).toEqual(ops[0]);
		expect(out[1]?.args.start).toBe("40f");
		expect(out[2]).toEqual(ops[2]);
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
	test("Add text is 3 s at the playhead and leaves the track to the core", () => {
		expect(
			textAddOperations({
				text: "Your text",
				playhead: 12,
			}),
		).toEqual([
			{
				op: "text.add",
				args: {
					start: "12f",
					duration: "3s",
					text: "Your text",
				},
			},
		]);
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
		expect(captionTextOperations("itm_a", "x y")).toEqual([
			{ op: "caption.set_words", args: { item_id: "itm_a", text: "x y" } },
		]);
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

describe("crossfade of neighbours on one track (VV-042)", () => {
	const first = { id: "itm_a", start: 0, duration: 30, track: "trk_1" };
	const second = { id: "itm_b", start: 30, duration: 30, track: "trk_1" };

	test("moves the arriving item to a free track", () => {
		expect(
			crossfadeOperations(second, first, "0.5s", {
				freeTrack: "trk_2",
				newTrackId: () => "trk_new",
			}),
		).toEqual([
			{
				op: "item.crossfade",
				args: {
					from_item_id: "itm_a",
					to_item_id: "itm_b",
					duration: "0.5s",
					track_id: "trk_2",
				},
			},
		]);
	});

	test("adds a visual track in the same batch when none is free", () => {
		expect(
			crossfadeOperations(first, second, "0.5s", {
				freeTrack: null,
				newTrackId: () => "trk_new",
			}),
		).toEqual([
			{ op: "track.add", args: { kind: "visual", id: "trk_new" } },
			{
				op: "item.crossfade",
				args: {
					from_item_id: "itm_a",
					to_item_id: "itm_b",
					duration: "0.5s",
					track_id: "trk_new",
				},
			},
		]);
	});

	test("items of different tracks send no track_id", () => {
		const other = { ...second, track: "trk_2" };
		const ops = crossfadeOperations(first, other, "0.5s", {
			freeTrack: "trk_3",
			newTrackId: () => "trk_new",
		});
		expect(ops).toHaveLength(1);
		expect(ops[0]?.args).not.toHaveProperty("track_id");
	});
});

describe("reframe builder (VV-038)", () => {
	test("sends only the aspect preset", () => {
		expect(reframeOperations("4:5")).toEqual([
			{ op: "settings.reframe", args: { aspect: "4:5" } },
		]);
	});
});

describe("track builders (SDD-002 C.6 to C.10)", () => {
	test("add names exactly one placement", () => {
		expect(trackAddOperations("audio", { above: "trk_a" })).toEqual([
			{ op: "track.add", args: { kind: "audio", above: "trk_a" } },
		]);
	});

	test("update sends only the changed fields", () => {
		expect(trackUpdateOperations("trk_a", { muted: true })).toEqual([
			{ op: "track.update", args: { track_id: "trk_a", muted: true } },
		]);
	});

	test("move names exactly one placement", () => {
		expect(trackMoveOperations("trk_a", { below: "trk_b" })).toEqual([
			{ op: "track.move", args: { track_id: "trk_a", below: "trk_b" } },
		]);
	});

	test("remove carries with_items only after the confirmation", () => {
		expect(trackRemoveOperations("trk_a", false)[0]?.args).toEqual({
			track_id: "trk_a",
		});
		expect(trackRemoveOperations("trk_a", true)[0]?.args).toEqual({
			track_id: "trk_a",
			with_items: true,
		});
	});

	test("the caption style merges unless replace is set", () => {
		expect(
			trackCaptionStyleOperations("trk_a", { size: 90 }, false)[0]?.args,
		).toEqual({ track_id: "trk_a", style: { size: 90 } });
		expect(
			trackCaptionStyleOperations("trk_a", { size: 90 }, true)[0]?.args,
		).toEqual({ track_id: "trk_a", style: { size: 90 }, replace: true });
	});
});

describe("project builders (SDD-002 C.1 to C.3)", () => {
	test("settings.set sends only the fields given", () => {
		expect(settingsSetOperations({ width: 1080, height: 1080 })).toEqual([
			{ op: "settings.set", args: { width: 1080, height: 1080 } },
		]);
	});

	test("a transparent background is settings.set", () => {
		expect(settingsSetOperations({ background: "transparent" })).toEqual([
			{ op: "settings.set", args: { background: "transparent" } },
		]);
	});

	test("the frame rate goes as a string", () => {
		expect(settingsFpsOperations("30000/1001")).toEqual([
			{ op: "settings.set_fps", args: { fps: "30000/1001" } },
		]);
	});

	test("rename sends the name as typed", () => {
		expect(projectRenameOperations(" A ")).toEqual([
			{ op: "project.rename", args: { name: " A " } },
		]);
	});
});

describe("clip section builders (SDD-002 C.19, C.24 to C.27)", () => {
	test("audio sends only the field that changed, fades with their unit", () => {
		expect(audioOperations("itm_a", { fade_in: "0.5s" })[0]?.args).toEqual({
			item_id: "itm_a",
			fade_in: "0.5s",
		});
		expect(audioOperations("itm_a", { audio_muted: true })[0]?.args).toEqual({
			item_id: "itm_a",
			audio_muted: true,
		});
	});

	test("framing names one crop side, the core merges the rest", () => {
		expect(
			framingOperations("itm_a", { crop: { left: 0.1 } })[0]?.args,
		).toEqual({ item_id: "itm_a", crop: { left: 0.1 } });
	});

	test("speed carries the x suffix and the ripple only when asked", () => {
		expect(speedOperations("itm_a", 1.5, false)[0]?.args).toEqual({
			item_id: "itm_a",
			speed: "1.5x",
		});
		expect(speedOperations("itm_a", 0.25, true)[0]?.args).toEqual({
			item_id: "itm_a",
			speed: "0.25x",
			ripple: "track",
		});
	});

	test("colour sends only the slider that was released", () => {
		expect(colorOperations("itm_a", { saturation: 1.4 })[0]?.args).toEqual({
			item_id: "itm_a",
			saturation: 1.4,
		});
	});

	test("white balance, hue, flip, enabled and denoise send only what changed", () => {
		expect(
			colorOperations("itm_a", { temperature: 0.5, tint: -0.2, hue: 30 })[0],
		).toEqual({
			op: "clip.set_color",
			args: { item_id: "itm_a", temperature: 0.5, tint: -0.2, hue: 30 },
		});
		expect(flipOperations("itm_a", { vertical: true })[0]).toEqual({
			op: "clip.set_flip",
			args: { item_id: "itm_a", vertical: true },
		});
		expect(enabledOperations("itm_a", false)[0]).toEqual({
			op: "item.set_enabled",
			args: { item_id: "itm_a", enabled: false },
		});
		expect(audioOperations("itm_a", { denoise: 0.4 })[0]?.args).toEqual({
			item_id: "itm_a",
			denoise: 0.4,
		});
		expect(audioOperations("itm_a", { voice: "warm" })[0]?.args).toEqual({
			item_id: "itm_a",
			voice: "warm",
		});
	});

	test("filter and blur send one field each", () => {
		expect(filterOperations("itm_a", { filter: "sepia" })).toEqual([
			{ op: "clip.set_filter", args: { item_id: "itm_a", filter: "sepia" } },
		]);
		expect(filterOperations("itm_a", { amount: 0.4 })[0]?.args).toEqual({
			item_id: "itm_a",
			amount: 0.4,
		});
		expect(blurOperations("itm_a", 12.5)).toEqual([
			{ op: "clip.set_blur", args: { item_id: "itm_a", radius: 12.5 } },
		]);
	});

	test("ducking sends the choices and never the speech ranges", () => {
		const args = duckOperations("itm_m", {
			speechItemIds: ["itm_s"],
			gainDb: -12,
			attack: "0.2s",
			release: "0.5s",
		})[0]?.args;
		expect(args).toEqual({
			item_id: "itm_m",
			speech_item_ids: ["itm_s"],
			gain_db: -12,
			attack: "0.2s",
			release: "0.5s",
		});
		expect(args).not.toHaveProperty("resolved");
	});

	test("beat markers send the choices, leave an empty label out and never the beats", () => {
		const args = markBeatsOperations("itm_m", {
			every: 2,
			label: "",
			replace: false,
		})[0]?.args;
		expect(args).toEqual({ item_id: "itm_m", every: 2, replace: false });
		expect(
			markBeatsOperations("itm_m", {
				every: 1,
				label: "beat",
				replace: true,
			})[0]?.args,
		).toEqual({ item_id: "itm_m", every: 1, replace: true, label: "beat" });
	});

	test("font add names the family and its files; remove names the family", () => {
		const hash = `sha256:${"ab".repeat(32)}`;
		expect(fontAddOperations("Brand Sans", [{ weight: 700, hash }])).toEqual([
			{
				op: "font.add",
				args: { family: "Brand Sans", files: [{ weight: 700, hash }] },
			},
		]);
		expect(fontRemoveOperations("Brand Sans")).toEqual([
			{ op: "font.remove", args: { family: "Brand Sans" } },
		]);
	});

	test("silences send the choices and never the ranges", () => {
		const args = silenceOperations("itm_a", {
			minSilence: "0.4s",
			padding: "0s",
			ripple: false,
		})[0]?.args;
		expect(args).toEqual({
			item_id: "itm_a",
			min_silence: "0.4s",
			padding: "0s",
			ripple: "none",
		});
		expect(args).not.toHaveProperty("resolved");
	});
});

describe("shape builders", () => {
	test("Add shape is 3 s at the playhead and leaves the track to the core", () => {
		const ops = shapeAddOperations({
			shape: "rounded_rect",
			playhead: 5,
		});
		expect(ops.map((o) => o.op)).toEqual(["shape.add"]);
		expect(ops[0]?.args).toEqual({
			start: "5f",
			duration: "3s",
			shape: "rounded_rect",
			width: 400,
			height: 240,
		});
	});

	test("a shape field is one partial request", () => {
		expect(shapeSetOperations("itm_a", { fill: "#FF0000" })).toEqual([
			{ op: "shape.set", args: { item_id: "itm_a", fill: "#FF0000" } },
		]);
	});
});

describe("template builders", () => {
	test("a slot mark carries the whole slot, with the policy for media only", () => {
		expect(
			slotMarkOperations("itm_a", {
				name: "title",
				description: "Headline",
				required: false,
			}),
		).toEqual([
			{
				op: "slot.mark",
				args: {
					item_id: "itm_a",
					name: "title",
					description: "Headline",
					required: false,
				},
			},
		]);
		expect(
			slotMarkOperations("itm_a", {
				name: "hero",
				description: "",
				required: true,
				durationPolicy: "stretch_timeline",
			})[0]?.args.duration_policy,
		).toBe("stretch_timeline");
	});

	test("unmark, fill and template info are one command each", () => {
		expect(slotUnmarkOperations("itm_a")).toEqual([
			{ op: "slot.unmark", args: { item_id: "itm_a" } },
		]);
		expect(slotFillOperations("title", { text: "x" })).toEqual([
			{ op: "slot.fill", args: { name: "title", text: "x" } },
		]);
		expect(slotFillOperations("hero", { asset_id: "ast_a" })).toEqual([
			{ op: "slot.fill", args: { name: "hero", asset_id: "ast_a" } },
		]);
		expect(templateInfoOperations({ colors: { brand: null } })).toEqual([
			{ op: "template.set_info", args: { colors: { brand: null } } },
		]);
	});
});

describe("nextSlotName", () => {
	test("is the first slot_N no item uses", () => {
		expect(nextSlotName([])).toBe("slot_1");
		expect(nextSlotName(["slot_1", undefined, "intro", "slot_3"])).toBe(
			"slot_2",
		);
	});
});

describe("volumeStepOperations", () => {
	test("moves each volume by the decibels, to 3 decimals, one set_audio each", () => {
		expect(
			volumeStepOperations(
				[
					{ id: "a", volume: 1 },
					{ id: "b", volume: 0.5 },
				],
				0.1,
			),
		).toEqual([
			{ op: "item.set_audio", args: { item_id: "a", volume: 1.012 } },
			{ op: "item.set_audio", args: { item_id: "b", volume: 0.506 } },
		]);
	});

	test("stays inside 0 to 4 and skips what does not change", () => {
		expect(volumeStepOperations([{ id: "a", volume: 4 }], 0.1)).toEqual([]);
		expect(volumeStepOperations([{ id: "a", volume: 0 }], -0.1)).toEqual([]);
		expect(volumeStepOperations([{ id: "a", volume: 3.999 }], 0.1)).toEqual([
			{ op: "item.set_audio", args: { item_id: "a", volume: 4 } },
		]);
	});
});

describe("slide, roll and slip builders (VV-036)", () => {
	test("slide sends the new start with slide true, leader first", () => {
		expect(
			slideOperations(
				[
					{ id: "a", start: 0, duration: 10 },
					{ id: "b", start: 10, duration: 10 },
				],
				2,
			),
		).toEqual([
			{ op: "item.move", args: { item_id: "b", start: "12f", slide: true } },
			{ op: "item.move", args: { item_id: "a", start: "2f", slide: true } },
		]);
	});

	test("roll moves the end edge with roll true", () => {
		expect(rollOperations([{ id: "a", start: 4, duration: 10 }], -1)).toEqual([
			{
				op: "item.trim",
				args: { item_id: "a", edge: "end", to: "13f", roll: true },
			},
		]);
	});

	test("slip sends the new source_in, never below 0", () => {
		expect(slipOperations([{ id: "a", sourceIn: 0 }], -5)).toEqual([
			{ op: "item.slip", args: { item_id: "a", source_in: "0f" } },
		]);
		expect(slipOperations([{ id: "a", sourceIn: 30 }], 1)).toEqual([
			{ op: "item.slip", args: { item_id: "a", source_in: "31f" } },
		]);
	});
});
