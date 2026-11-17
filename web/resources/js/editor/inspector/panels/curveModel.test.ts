import { describe, expect, test } from "vitest";

import {
	addPoint,
	addPointInWidestGap,
	MAX_CURVE_POINTS,
	movePoint,
	NEUTRAL_CURVE,
	removePoint,
	sameCurve,
} from "./curveModel";

describe("curve editing", () => {
	test("a moved point keeps its x between its neighbours and its y in 0..1", () => {
		const three = [
			{ x: 0, y: 0 },
			{ x: 0.5, y: 0.5 },
			{ x: 1, y: 1 },
		];
		expect(movePoint(three, 1, 1.5, 1.4)[1]).toEqual({ x: 0.999, y: 1 });
		expect(movePoint(three, 1, -3, -1)[1]).toEqual({ x: 0.001, y: 0 });
		expect(movePoint(three, 0, 0.3, 0.2)[0]).toEqual({ x: 0.3, y: 0.2 });
		expect(three[1]).toEqual({ x: 0.5, y: 0.5 });
	});

	test("a point is added in order, and refused on an existing x or at 8 points", () => {
		expect(addPoint(NEUTRAL_CURVE, 0.25, 0.4)).toEqual([
			{ x: 0, y: 0 },
			{ x: 0.25, y: 0.4 },
			{ x: 1, y: 1 },
		]);
		expect(addPoint(NEUTRAL_CURVE, 1, 0.5)).toHaveLength(2);
		const full = Array.from({ length: MAX_CURVE_POINTS }, (_, at) => ({
			x: at / 10,
			y: at / 10,
		}));
		expect(addPoint(full, 0.95, 0.5)).toHaveLength(MAX_CURVE_POINTS);
	});

	test("Add point splits the widest gap at its middle", () => {
		const points = [
			{ x: 0, y: 0 },
			{ x: 0.2, y: 0.1 },
			{ x: 1, y: 1 },
		];
		expect(addPointInWidestGap(points)[2]).toEqual({ x: 0.6, y: 0.55 });
	});

	test("a point is removed down to two", () => {
		const three = [
			{ x: 0, y: 0 },
			{ x: 0.5, y: 0.7 },
			{ x: 1, y: 1 },
		];
		expect(removePoint(three, 1)).toEqual(NEUTRAL_CURVE);
		expect(removePoint(NEUTRAL_CURVE, 0)).toEqual(NEUTRAL_CURVE);
		expect(sameCurve(three, three)).toBe(true);
		expect(sameCurve(three, NEUTRAL_CURVE)).toBe(false);
	});
});
