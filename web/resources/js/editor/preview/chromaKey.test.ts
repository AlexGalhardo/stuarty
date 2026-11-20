import { describe, expect, it } from "vitest";

import {
	CHROMA_FRAGMENT_SOURCE,
	chromaSettings,
	keepOf,
	keyPixel,
} from "./chromaKey";

const key = (color: string, similarity: number, smoothness = 0, spill = 0) =>
	chromaSettings({ color, similarity, smoothness, spill });

// The vectors of engine/compositor/chroma_test.go (SDD-004 section 3.7).
describe("chroma key reference", () => {
	it("removes the key colour and keeps the colours far from it", () => {
		const s = key("#00FF00", 0.3);
		expect(keyPixel(s, [0, 255, 0, 255])).toEqual([0, 0, 0, 0]);
		expect(keyPixel(s, [255, 0, 0, 255])).toEqual([255, 0, 0, 255]);
		expect(keyPixel(s, [0, 0, 255, 255])).toEqual([0, 0, 255, 255]);
		expect(keyPixel(s, [30, 230, 30, 255])).toEqual([0, 0, 0, 0]);
		expect(keyPixel(s, [60, 200, 60, 255])).toEqual([60, 200, 60, 255]);
	});

	it("fades the soft edge linearly and monotonically", () => {
		const s = key("#00FF00", 0.1, 0.4);
		expect(keepOf(s, 0, 255, 0)).toBe(0);
		expect(keepOf(s, 255, 0, 0)).toBe(255);
		let prev = -1;
		let ramp = 0;
		for (let g = 255; g >= 0; g -= 5) {
			const f = keepOf(s, 0, g, 0);
			expect(f).toBeGreaterThanOrEqual(prev);
			if (f > 0 && f < 255) ramp++;
			prev = f;
		}
		expect(ramp).toBeGreaterThanOrEqual(5);
	});

	it("has no ramp with smoothness 0", () => {
		const s = key("#00FF00", 0.1);
		for (let g = 255; g >= 0; g--) {
			const f = keepOf(s, 0, g, 0);
			expect(f === 0 || f === 255).toBe(true);
		}
	});

	it("takes the key colour out of what stays with spill", () => {
		const px = [200, 190, 120, 255] as const;
		expect(keyPixel(key("#00FF00", 0.05), px)).toEqual([200, 190, 120, 255]);
		expect(keyPixel(key("#00FF00", 0.05, 0, 1), px)).toEqual([
			200, 160, 120, 255,
		]);
		expect(keyPixel(key("#00FF00", 0.05, 0, 0.5), px)[1]).toBe(175);
		// A grey key has no dominant channel: nothing to despill.
		expect(keyPixel(key("#808080", 0.01, 0, 1), px)).toEqual([
			200, 190, 120, 255,
		]);
	});

	it("keys premultiplied pixels through their straight colour", () => {
		const s = key("#00FF00", 0.3);
		expect(keyPixel(s, [0, 128, 0, 128])).toEqual([0, 0, 0, 0]);
		expect(keyPixel(s, [128, 0, 0, 128])).toEqual([128, 0, 0, 128]);
		expect(keyPixel(s, [0, 0, 0, 0])).toEqual([0, 0, 0, 0]);
	});
});

// VV-093: a WebGL context that compiles a fragment shader as GLSL ES 1.00 refuses an integer modulus and an
// index by a variable ("integer modulus operator supported in GLSL ES 3.00 and above only"); the program is
// then not valid and the keyed layer draws nothing at all, subject included.
describe("chroma key shader", () => {
	it("uses no integer modulus and no vector indexed by a variable", () => {
		expect(CHROMA_FRAGMENT_SOURCE).not.toMatch(/%/);
		expect(CHROMA_FRAGMENT_SOURCE).not.toMatch(/\w\[\s*[A-Za-z_]/);
	});
});
