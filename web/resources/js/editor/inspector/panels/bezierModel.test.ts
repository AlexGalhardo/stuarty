import { describe, expect, test } from "vitest";

import {
	clampX,
	clampY,
	curveOf,
	curvePath,
	fromViewX,
	fromViewY,
	moveHandle,
	sameCurve,
	setNumber,
	toViewX,
	toViewY,
} from "./bezierModel";

describe("bezier model", () => {
	test("a keyframe has a curve only with four numbers", () => {
		expect(curveOf(undefined)).toBeNull();
		expect(curveOf([0.1, 0.2])).toBeNull();
		expect(curveOf([0.1, 0.2, 0.3, 0.4])).toEqual([0.1, 0.2, 0.3, 0.4]);
	});

	test("the x of a control point stays from 0 to 1 and the y from -2 to 3", () => {
		expect(clampX(1.4)).toBe(1);
		expect(clampX(-0.2)).toBe(0);
		expect(clampY(3.5)).toBe(3);
		expect(clampY(-2.5)).toBe(-2);
		expect(clampX(0.123456)).toBe(0.123);
	});

	test("moving a handle leaves the other one", () => {
		expect(moveHandle([0.1, 0.2, 0.3, 0.4], 1, 0.9, 1.5)).toEqual([
			0.1, 0.2, 0.9, 1.5,
		]);
		expect(moveHandle([0.1, 0.2, 0.3, 0.4], 0, 2, -5)).toEqual([
			1, -2, 0.3, 0.4,
		]);
	});

	test("setting one number clamps by its axis", () => {
		expect(setNumber([0.1, 0.2, 0.3, 0.4], 0, 7)).toEqual([1, 0.2, 0.3, 0.4]);
		expect(setNumber([0.1, 0.2, 0.3, 0.4], 3, 7)).toEqual([0.1, 0.2, 0.3, 3]);
	});

	test("the plot maps the numbers and back", () => {
		for (const x of [0, 0.25, 1]) expect(fromViewX(toViewX(x))).toBeCloseTo(x);
		for (const y of [-1, 0, 0.5, 2]) {
			expect(fromViewY(toViewY(y))).toBeCloseTo(y);
		}
		expect(toViewY(3)).toBe(toViewY(2));
	});

	test("the path goes from the corner (0, 0) to the corner (1, 1)", () => {
		const path = curvePath([0.25, 0.1, 0.25, 1]);
		expect(path.startsWith(`M${toViewX(0)} ${toViewY(0)}`)).toBe(true);
		expect(path.endsWith(`${toViewX(1)} ${toViewY(1)}`)).toBe(true);
		expect(path).toContain("C");
	});

	test("two curves are the same when the four numbers are", () => {
		expect(sameCurve([0, 0, 1, 1], [0, 0, 1, 1])).toBe(true);
		expect(sameCurve([0, 0, 1, 1], [0, 0, 1, 0.9])).toBe(false);
	});
});
