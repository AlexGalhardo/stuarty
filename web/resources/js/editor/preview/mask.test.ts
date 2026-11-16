import { describe, expect, it } from "vitest";

import type { PlanMask } from "../api/types";
import { coverageSize, coverageTexels, maskCoverage, maskShape } from "./mask";

const rect: PlanMask = {
	shape: "rect",
	x: 0.25,
	y: 0.25,
	width: 0.5,
	height: 0.5,
	feather: 0,
};
const circle: PlanMask = {
	shape: "ellipse",
	x: 0,
	y: 0,
	width: 1,
	height: 1,
	feather: 0,
};

// The pixel centre of output pixel (x, y) in 24.8 (SDD-004 section 3.9 step 1).
const at = (x: number, y: number): [number, number] => [
	x * 256 + 128,
	y * 256 + 128,
];
const cov = (
	mask: PlanMask,
	w: number,
	h: number,
	scale: number,
	x: number,
	y: number,
) => maskCoverage(maskShape(mask, w, h, scale), ...at(x, y));

// The values of section 3.9 worked by hand with the integer rules of the spec.
describe("mask coverage reference", () => {
	it("rounds the shape to 24.8", () => {
		expect(maskShape(rect, 100, 100, 1)).toMatchObject({
			left: 6400,
			top: 6400,
			a: 6400,
			b: 6400,
			feather: 0,
		});
	});

	it("reads a feather the plan left out as 0", () => {
		const { feather: _omitted, ...plan } = rect;
		expect(maskShape(plan, 100, 100, 1).feather).toBe(0);
	});

	it("is hard on the edge of a rectangle without feather", () => {
		expect(cov(rect, 100, 100, 1, 50, 50)).toBe(255);
		expect(cov(rect, 100, 100, 1, 25, 50)).toBe(255);
		expect(cov(rect, 100, 100, 1, 24, 50)).toBe(0);
		expect(cov(rect, 100, 100, 1, 74, 50)).toBe(255);
		expect(cov(rect, 100, 100, 1, 75, 50)).toBe(0);
	});

	it("ramps linearly across the edge with feather, centred on it", () => {
		const soft = { ...rect, feather: 8 };
		expect(cov(soft, 100, 100, 1, 25, 50)).toBe(143);
		expect(cov(soft, 100, 100, 1, 24, 50)).toBe(112);
		expect(cov(soft, 100, 100, 1, 50, 50)).toBe(255);
		expect(cov(soft, 100, 100, 1, 0, 0)).toBe(0);
	});

	it("scales the feather with the output scale", () => {
		expect(maskShape({ ...rect, feather: 8 }, 100, 100, 0.5).feather).toBe(
			1024,
		);
	});

	it("inverts the coverage", () => {
		const soft = { ...rect, feather: 8, invert: true };
		expect(cov(soft, 100, 100, 1, 25, 50)).toBe(112);
		expect(cov(soft, 100, 100, 1, 50, 50)).toBe(0);
		expect(cov(soft, 100, 100, 1, 0, 0)).toBe(255);
	});

	it("covers the inside of an ellipse and not its corners", () => {
		expect(cov(circle, 100, 100, 1, 50, 50)).toBe(255);
		expect(cov(circle, 100, 100, 1, 0, 0)).toBe(0);
		expect(cov(circle, 100, 100, 1, 99, 50)).toBe(255);
	});

	it("feathers an ellipse on its axis like a rectangle edge", () => {
		expect(cov({ ...circle, feather: 8 }, 100, 100, 1, 99, 50)).toBe(143);
	});
});

describe("coverage texels", () => {
	it("are the pixels of the export when the grid is the rectangle", () => {
		const soft = { ...rect, feather: 8 };
		const texels = coverageTexels(soft, 100, 100, 1, 100, 100);
		for (const [x, y] of [
			[25, 50],
			[24, 50],
			[50, 50],
			[0, 0],
		] as const) {
			const i = (y * 100 + x) * 4;
			const want = cov(soft, 100, 100, 1, x, y);
			expect([texels[i], texels[i + 1], texels[i + 2], texels[i + 3]]).toEqual([
				want,
				want,
				want,
				want,
			]);
		}
	});

	it("shrinks a large layer to the texture cap, keeping the aspect", () => {
		expect(coverageSize(100, 50)).toEqual({ width: 100, height: 50 });
		expect(coverageSize(1920, 1080)).toEqual({ width: 640, height: 360 });
		expect(coverageSize(1080, 1920)).toEqual({ width: 360, height: 640 });
	});
});
