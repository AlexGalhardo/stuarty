import { describe, expect, test } from "vitest";

import { keyframeToggle, resetOf } from "./TransformSection";

describe("resetOf", () => {
	test("X and Y have none: the registry gives them no clearing value", () => {
		expect(resetOf("x", ["540"], false)).toBeNull();
		expect(resetOf("y", ["960"], false)).toBeNull();
	});

	test("uses the values the core documents as clearing the property", () => {
		expect(resetOf("scale", ["2"], false)).toEqual({
			value: 1,
			disabled: false,
		});
		expect(resetOf("rotation", ["15"], false)?.value).toBe(0);
		expect(resetOf("opacity", ["0.5"], false)?.value).toBe(1);
	});

	test("is off at the default for every item, and for a keyframed property", () => {
		expect(resetOf("scale", ["1", "1"], false)?.disabled).toBe(true);
		expect(resetOf("scale", ["1", "2"], false)?.disabled).toBe(false);
		expect(resetOf("scale", ["2"], true)?.disabled).toBe(true);
	});
});

describe("keyframeToggle", () => {
	const item = (id: string, keyframes?: { t: number; value: number }[]) => ({
		id,
		start: 10,
		duration: 100,
		keyframes: keyframes === undefined ? undefined : { opacity: keyframes },
	});

	test("adds a keyframe at the playhead with the shown value for each item", () => {
		const toggle = keyframeToggle(
			"opacity",
			[item("itm_a"), item("itm_b")],
			["0.5", "1"],
			30,
		);
		expect(toggle?.on).toBe(false);
		expect(toggle?.operations).toEqual([
			{
				op: "keyframe.set",
				args: { item_id: "itm_a", property: "opacity", at: "30f", value: 0.5 },
			},
			{
				op: "keyframe.set",
				args: { item_id: "itm_b", property: "opacity", at: "30f", value: 1 },
			},
		]);
	});

	test("removes the keyframes when every item has one at the playhead", () => {
		const toggle = keyframeToggle(
			"opacity",
			[item("itm_a", [{ t: 20, value: 0.5 }])],
			["0.5"],
			30,
		);
		expect(toggle?.on).toBe(true);
		expect(toggle?.operations).toEqual([
			{
				op: "keyframe.remove",
				args: { item_id: "itm_a", property: "opacity", at: "30f" },
			},
		]);
	});

	test("is unavailable outside an item or without a shown value", () => {
		expect(keyframeToggle("opacity", [item("itm_a")], ["1"], 5)).toBeNull();
		expect(keyframeToggle("opacity", [item("itm_a")], [""], 30)).toBeNull();
	});
});
