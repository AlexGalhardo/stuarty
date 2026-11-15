import { describe, expect, test } from "vitest";

import type { LayerGeometry } from "../types";
import { containsPoint, hitTest } from "./hitTest";

const layer = (
	item: string,
	cx: number,
	cy: number,
	w: number,
	h: number,
	rotation = 0,
): LayerGeometry => ({ item, cx, cy, w, h, rotation, scale: 1, opacity: 1 });

describe("hitTest", () => {
	test("an unrotated rectangle contains its own area, edges included", () => {
		const l = layer("a", 100, 100, 40, 20);
		expect(containsPoint(l, { x: 100, y: 100 })).toBe(true);
		expect(containsPoint(l, { x: 120, y: 110 })).toBe(true);
		expect(containsPoint(l, { x: 121, y: 100 })).toBe(false);
		expect(containsPoint(l, { x: 100, y: 111 })).toBe(false);
	});

	test("a rotated rectangle is tested in its own axes", () => {
		// 100 wide, 20 high, turned a quarter clockwise: it is now 20 wide, 100 high.
		const l = layer("a", 0, 0, 100, 20, 90);
		expect(containsPoint(l, { x: 0, y: 45 })).toBe(true);
		expect(containsPoint(l, { x: 45, y: 0 })).toBe(false);
		// A 45 degree turn: the corner of the unrotated box is outside, the diagonal inside.
		const d = layer("d", 0, 0, 100, 20, 45);
		expect(containsPoint(d, { x: 30, y: 30 })).toBe(true);
		expect(containsPoint(d, { x: 45, y: -45 })).toBe(false);
		expect(containsPoint(d, { x: 50, y: 0 })).toBe(false);
	});

	test("the topmost layer, the last in plan order, wins an overlap", () => {
		const layers = [
			layer("bottom", 50, 50, 100, 100),
			layer("top", 60, 60, 40, 40),
		];
		expect(hitTest(layers, { x: 60, y: 60 })).toBe("top");
		expect(hitTest(layers, { x: 10, y: 10 })).toBe("bottom");
	});

	test("a point that hits nothing is null, which clears the selection", () => {
		expect(
			hitTest([layer("a", 50, 50, 10, 10)], { x: 500, y: 500 }),
		).toBeNull();
		expect(hitTest([], { x: 0, y: 0 })).toBeNull();
	});
});
