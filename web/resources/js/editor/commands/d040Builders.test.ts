import { describe, expect, test } from "vitest";

import {
	colorOperations,
	colorResetChanges,
	constantSpeedOperations,
	defaultEffect,
	EFFECT_KINDS,
	EFFECT_PARAMS,
	effectsOperations,
	extractAudioOperations,
	freezeFrameOperations,
	insertCurvePoint,
	keyframeAt,
	keyframeSetOperations,
	linkOperations,
	PASTE_GROUPS,
	pasteAttributesOperations,
	reverseOperations,
	shownColourKeyframe,
	speedCurveOperations,
	trackUpdateOperations,
	transitionOperations,
	unlinkOperations,
} from "./builders";

describe("the builders of the D-040 commands", () => {
	test("a curve goes as points with the x suffix and 3 decimals", () => {
		expect(
			speedCurveOperations(
				"itm_a",
				[
					{ t: 0, speed: 1 },
					{ t: 0.5004, speed: 0.3333 },
					{ t: 1, speed: 4 },
				],
				false,
			),
		).toEqual([
			{
				op: "clip.set_speed",
				args: {
					item_id: "itm_a",
					curve: [
						{ t: 0, speed: "1x" },
						{ t: 0.5, speed: "0.333x" },
						{ t: 1, speed: "4x" },
					],
				},
			},
		]);
	});

	test("a curve takes the ripple only when asked", () => {
		const ops = speedCurveOperations(
			"itm_a",
			[
				{ t: 0, speed: 1 },
				{ t: 1, speed: 2 },
			],
			true,
		);
		expect(ops[0]?.args).toMatchObject({ ripple: "track" });
	});

	test("removing a curve sends the stored mean as a fraction", () => {
		expect(
			constantSpeedOperations("itm_a", { num: 7, den: 3 })[0]?.args,
		).toEqual({
			item_id: "itm_a",
			speed: { num: 7, den: 3 },
		});
	});

	test("a new point goes in the middle of the widest gap", () => {
		expect(
			insertCurvePoint([
				{ t: 0, speed: 1 },
				{ t: 0.2, speed: 2 },
				{ t: 1, speed: 4 },
			]),
		).toEqual([
			{ t: 0, speed: 1 },
			{ t: 0.2, speed: 2 },
			{ t: 0.6, speed: 3 },
			{ t: 1, speed: 4 },
		]);
	});

	test("reverse is sent alone", () => {
		expect(reverseOperations("itm_a", true)).toEqual([
			{ op: "clip.set_speed", args: { item_id: "itm_a", reverse: true } },
		]);
		expect(reverseOperations("itm_a", false)[0]?.args).toEqual({
			item_id: "itm_a",
			reverse: false,
		});
	});

	test("effects are the whole list, and [] clears it", () => {
		expect(effectsOperations("itm_a", [])).toEqual([
			{ op: "clip.set_effects", args: { item_id: "itm_a", effects: [] } },
		]);
		expect(
			effectsOperations("itm_a", [defaultEffect("vignette")])[0]?.args,
		).toEqual({
			item_id: "itm_a",
			effects: [{ kind: "vignette", amount: 0.5, softness: 0.5 }],
		});
	});

	test("every kind has the defaults and ranges of SDD-002 C.68", () => {
		expect(EFFECT_KINDS).toHaveLength(6);
		expect(defaultEffect("glow")).toEqual({
			kind: "glow",
			amount: 0.5,
			radius: 12,
			threshold: 0.6,
		});
		expect(defaultEffect("shake")).toEqual({
			kind: "shake",
			amount: 0.3,
			speed: 8,
			seed: 0,
		});
		expect(EFFECT_PARAMS.grain.map((p) => p.name)).toEqual([
			"amount",
			"size",
			"seed",
		]);
		for (const kind of EFFECT_KINDS) {
			for (const param of EFFECT_PARAMS[kind]) {
				expect(param.default).toBeGreaterThanOrEqual(param.min);
				expect(param.default).toBeLessThanOrEqual(param.max);
			}
		}
	});

	test("link and unlink take the ids as given", () => {
		expect(linkOperations(["itm_a", "itm_b"])).toEqual([
			{ op: "item.link", args: { item_ids: ["itm_a", "itm_b"] } },
		]);
		expect(unlinkOperations(["itm_a"])[0]).toEqual({
			op: "item.unlink",
			args: { item_ids: ["itm_a"] },
		});
	});

	test("extract audio uses the track given, or adds one first", () => {
		expect(
			extractAudioOperations({
				itemId: "itm_a",
				audioTrack: "trk_a",
				newTrackId: () => "trk_new",
			}),
		).toEqual([
			{
				op: "item.extract_audio",
				args: { item_id: "itm_a", track_id: "trk_a" },
			},
		]);
		expect(
			extractAudioOperations({
				itemId: "itm_a",
				audioTrack: null,
				newTrackId: () => "trk_new",
			}),
		).toEqual([
			{ op: "track.add", args: { kind: "audio", id: "trk_new" } },
			{
				op: "item.extract_audio",
				args: { item_id: "itm_a", track_id: "trk_new" },
			},
		]);
	});

	test("a transition sends the colour and the direction only when given", () => {
		expect(
			transitionOperations({
				itemId: "itm_a",
				edge: "in",
				kind: "dip",
				color: "#ff0000",
			})[0]?.args,
		).toEqual({
			item_id: "itm_a",
			edge: "in",
			kind: "dip",
			color: "#ff0000",
		});
		expect(
			transitionOperations({
				itemId: "itm_a",
				edge: "out",
				kind: "wipe",
				direction: "up",
			})[0]?.args,
		).toMatchObject({ kind: "wipe", direction: "up" });
		expect(
			transitionOperations({ itemId: "itm_a", edge: "in", kind: "fade" })[0]
				?.args,
		).toEqual({ item_id: "itm_a", edge: "in", kind: "fade" });
	});

	test("a freeze frame holds the picture at the playhead of the first item under it", () => {
		const items = [
			{ id: "itm_a", start: 0, duration: 30 },
			{ id: "itm_b", start: 10, duration: 40 },
		];
		expect(freezeFrameOperations(items, 20)).toEqual([
			{ op: "item.freeze_frame", args: { item_id: "itm_a", at: "20f" } },
		]);
		expect(freezeFrameOperations(items, 40)).toEqual([
			{ op: "item.freeze_frame", args: { item_id: "itm_b", at: "40f" } },
		]);
		// The playhead on an edge, or outside every item, sends nothing.
		expect(freezeFrameOperations(items, 0)).toEqual([]);
		expect(freezeFrameOperations(items, 50)).toEqual([]);
		expect(freezeFrameOperations(items, 90)).toEqual([]);
	});

	test("paste attributes copies every group from one item to the others", () => {
		expect(pasteAttributesOperations("itm_a", ["itm_b", "itm_c"])).toEqual([
			{
				op: "item.paste_attributes",
				args: {
					from_item_id: "itm_a",
					item_ids: ["itm_b", "itm_c"],
					attributes: [...PASTE_GROUPS],
				},
			},
		]);
		expect(pasteAttributesOperations("itm_a", [])).toEqual([]);
	});

	test("the magnet is one track.update with the new value", () => {
		expect(trackUpdateOperations("trk_a", { magnet: true })).toEqual([
			{ op: "track.update", args: { track_id: "trk_a", magnet: true } },
		]);
	});

	test("a lookup table goes by asset id and its intensity; 0 removes it", () => {
		expect(colorOperations("itm_a", { lut_asset: "ast_lut" })).toEqual([
			{
				op: "clip.set_color",
				args: { item_id: "itm_a", lut_asset: "ast_lut" },
			},
		]);
		expect(colorOperations("itm_a", { lut_intensity: 0 })[0]?.args).toEqual({
			item_id: "itm_a",
			lut_intensity: 0,
		});
	});

	test("an HSL band, a wheel and a curve are sent whole and replace the stored one", () => {
		expect(
			colorOperations("itm_a", {
				hsl: [{ range: "red", hue: 0.2, saturation: 0, lightness: -0.1 }],
				wheels: { shadows: { hue: 210, amount: 0.4, luma: 0 } },
				curves: {
					master: [
						{ x: 0, y: 0 },
						{ x: 0.5, y: 0.6 },
						{ x: 1, y: 1 },
					],
				},
			})[0]?.args,
		).toEqual({
			item_id: "itm_a",
			hsl: [{ range: "red", hue: 0.2, saturation: 0, lightness: -0.1 }],
			wheels: { shadows: { hue: 210, amount: 0.4, luma: 0 } },
			curves: {
				master: [
					{ x: 0, y: 0 },
					{ x: 0.5, y: 0.6 },
					{ x: 1, y: 1 },
				],
			},
		});
	});

	test("Reset sends the neutral value of what the item stores", () => {
		const none = { hsl: [], wheels: [], curves: [] } as const;
		expect(colorResetChanges("hsl", { ...none, hsl: ["red", "blue"] })).toEqual(
			{
				hsl: [
					{ range: "red", hue: 0, saturation: 0, lightness: 0 },
					{ range: "blue", hue: 0, saturation: 0, lightness: 0 },
				],
			},
		);
		expect(
			colorResetChanges("wheels", { ...none, wheels: ["midtones"] }),
		).toEqual({ wheels: { midtones: { hue: 0, amount: 0, luma: 0 } } });
		expect(
			colorResetChanges("curves", { ...none, curves: ["master", "green"] }),
		).toEqual({ curves: { master: [], green: [] } });
	});

	test("a colour keyframe is one keyframe.set at the frame, with the easing only when given", () => {
		expect(
			keyframeSetOperations({
				itemId: "itm_a",
				property: "brightness",
				frame: 12,
				value: 0.4,
			}),
		).toEqual([
			{
				op: "keyframe.set",
				args: {
					item_id: "itm_a",
					property: "brightness",
					at: "12f",
					value: 0.4,
				},
			},
		]);
		expect(
			keyframeSetOperations({
				itemId: "itm_a",
				property: "hue",
				frame: 12,
				value: 30,
				easing: "hold",
			})[0]?.args,
		).toMatchObject({ easing: "hold" });
	});

	test("an animated colour slider shows the keyframe at the playhead, else the latest before it, else the first", () => {
		const keys = [
			{ t: 10, value: 0.1 },
			{ t: 30, value: 0.5, easing: "hold" as const },
		];
		expect(keyframeAt(keys, 100, 130)).toEqual(keys[1]);
		expect(keyframeAt(keys, 100, 120)).toBeUndefined();
		expect(shownColourKeyframe(keys, 100, 130)?.value).toBe(0.5);
		expect(shownColourKeyframe(keys, 100, 125)?.value).toBe(0.1);
		expect(shownColourKeyframe(keys, 100, 105)?.value).toBe(0.1);
		expect(shownColourKeyframe([], 100, 105)).toBeUndefined();
	});
});
