import { describe, expect, test } from "vitest";
import type { Hunk } from "./client";
import { changedCount, sidesOf } from "./compare";

const hunks: Hunk[] = [
	{ op: "equal", text: "stays" },
	{ op: "removed", text: "gone" },
	{ op: "added", text: "new" },
];

describe("the sides of a comparison", () => {
	test("the revision compared from holds what stayed and what was taken out", () => {
		expect(sidesOf(hunks).from.map((h) => h.text)).toEqual(["stays", "gone"]);
	});

	test("the revision compared to holds what stayed and what was put in", () => {
		expect(sidesOf(hunks).to.map((h) => h.text)).toEqual(["stays", "new"]);
	});

	test("the order of the core is kept and nothing is added", () => {
		const sides = sidesOf(hunks);
		expect(sides.from.length + sides.to.length).toBe(hunks.length + 1);
		expect(sidesOf([])).toEqual({ from: [], to: [] });
	});

	test("two equal texts have nothing changed", () => {
		expect(changedCount([{ op: "equal", text: "a" }])).toBe(0);
		expect(changedCount(hunks)).toBe(2);
	});
});
