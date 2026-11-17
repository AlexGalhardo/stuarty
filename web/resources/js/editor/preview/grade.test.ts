import { describe, expect, test } from "vitest";

import {
	curveTable,
	curveTexels,
	gradeIsIdentity,
	gradeUniforms,
} from "./grade";

// The entries 0, 15, ..., 255 of the tables that engine/compositor/grade.go
// builds from the same points (SDD-004 section 3.14, integer form).
function sampled(t: Uint8Array | null): number[] {
	if (t === null) return [];
	return [...Array.from({ length: 18 }, (_, i) => t[i * 15]), t[255], t[254]];
}

describe("curve table", () => {
	test("is the table of the Go compositor for a master curve", () => {
		const t = curveTable([
			{ x: 0, y: 0.1 },
			{ x: 0.3, y: 0.2 },
			{ x: 0.7, y: 0.9 },
			{ x: 1, y: 1 },
		]);
		expect(sampled(t)).toEqual([
			26, 29, 31, 33, 39, 50, 68, 94, 124, 156, 186, 212, 231, 242, 247, 250,
			252, 255, 255, 255,
		]);
	});

	test("matches a second curve of the golden cases", () => {
		const t = curveTable([
			{ x: 0, y: 0.08 },
			{ x: 0.4, y: 0.3 },
			{ x: 0.75, y: 0.85 },
			{ x: 1, y: 1 },
		]);
		expect(sampled(t)).toEqual([
			20, 28, 34, 39, 46, 54, 65, 80, 100, 125, 152, 179, 203, 221, 233, 241,
			247, 255, 255, 254,
		]);
	});

	test("holds the first and the last value outside the points", () => {
		const t = curveTable([
			{ x: 0.2, y: 0.5 },
			{ x: 0.6, y: 0.1 },
		]);
		expect(sampled(t)).toEqual([
			128, 128, 128, 128, 119, 104, 89, 74, 59, 44, 29, 26, 26, 26, 26, 26, 26,
			26, 26, 26,
		]);
	});

	test("no points is no table", () => {
		expect(curveTable([])).toBeNull();
	});
});

describe("grade uniforms", () => {
	test("a grade with nothing set is the identity", () => {
		expect(gradeIsIdentity(gradeUniforms({}))).toBe(true);
		expect(
			gradeIsIdentity(
				gradeUniforms({
					hsl: [{ range: "red" }],
					wheels: { shadows: { hue: 10, amount: 0, luma: 0 } },
				}),
			),
		).toBe(true);
	});

	test("puts each band at the index of its range", () => {
		const u = gradeUniforms({
			hsl: [{ range: "blue", hue: 0.5, saturation: -1, lightness: 0.25 }],
		});
		expect([...u.flags]).toEqual([1, 0, 0, 0]);
		expect([...u.bands[5]]).toEqual([0.5, -1, 0.25, 0]);
		expect([...u.bands[0]]).toEqual([0, 0, 0, 0]);
	});

	test("a wheel tint has no luma and a hue of 0 pushes towards red", () => {
		const u = gradeUniforms({
			wheels: { highlights: { hue: 0, amount: 1, luma: 0.4 } },
		});
		const [r, g, b, luma] = u.wheels[2];
		expect(r + g + b).toBeCloseTo(0, 6);
		expect(r).toBeGreaterThan(0);
		expect(g).toBeLessThan(0);
		expect(luma).toBeCloseTo(0.1, 3);
	});

	test("the red curve goes through the master curve", () => {
		const texels = curveTexels({
			master: [
				{ x: 0, y: 0.5 },
				{ x: 1, y: 1 },
			],
			red: [
				{ x: 0, y: 1 },
				{ x: 1, y: 0 },
			],
		});
		// Master lifts 0 to 128, then red inverts it.
		expect(texels?.[0]).toBe(127);
		expect(texels?.[1]).toBe(128);
		expect(texels?.[3]).toBe(255);
	});
});
