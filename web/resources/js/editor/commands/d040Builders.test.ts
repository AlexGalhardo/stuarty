import { describe, expect, test } from "vitest";

import {
	constantSpeedOperations,
	defaultEffect,
	EFFECT_KINDS,
	EFFECT_PARAMS,
	effectsOperations,
	extractAudioOperations,
	linkOperations,
	reverseOperations,
	speedCurveOperations,
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
});
