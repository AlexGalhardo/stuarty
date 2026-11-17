import { describe, expect, test } from "vitest";

import executorSource from "./PixiExecutor.ts?raw";
import { type ShadowOutlineImage, shadowOutline } from "./shadowOutline";

function solid(
	w: number,
	h: number,
	px: [number, number, number, number],
): Uint8Array {
	const out = new Uint8Array(w * h * 4);
	for (let i = 0; i < w * h; i++) out.set(px, i * 4);
	return out;
}

function at(img: ShadowOutlineImage, x: number, y: number): number[] {
	const i = (y * img.width + x) * 4;
	return Array.from(img.pixels.subarray(i, i + 4));
}

const red = [255, 0, 0, 255] as [number, number, number, number];

describe("clip shadow and outline", () => {
	test("both off leaves the buffer and its size unchanged", () => {
		const src = solid(2, 2, [8, 16, 32, 64]);
		const before = src.slice();
		const got = shadowOutline(src, 2, 2, {}, 1, 2, 2);
		expect(got.pixels).toBe(src);
		expect(Array.from(src)).toEqual(Array.from(before));
		expect(got.width).toBe(2);
		expect(got.height).toBe(2);
		expect(got.shadow).toBeNull();
		expect(got.outline).toBeNull();
	});

	test("an outline of radius 1 paints the disc and keeps the source pixel", () => {
		const src = solid(1, 1, red);
		const got = shadowOutline(
			src,
			1,
			1,
			{ outline: { color: "#FFFFFF", width: 1 } },
			1,
			1,
			1,
		);
		expect(got.width).toBe(3);
		expect(got.height).toBe(3);
		expect(at(got, 1, 1)).toEqual([255, 0, 0, 255]);
		expect(at(got, 1, 0)).toEqual([255, 255, 255, 255]);
		expect(at(got, 0, 1)).toEqual([255, 255, 255, 255]);
		expect(at(got, 2, 1)).toEqual([255, 255, 255, 255]);
		expect(at(got, 1, 2)).toEqual([255, 255, 255, 255]);
		expect(at(got, 0, 0)).toEqual([0, 0, 0, 0]);
		expect(at(got, 2, 0)).toEqual([0, 0, 0, 0]);
		expect(at(got, 0, 2)).toEqual([0, 0, 0, 0]);
		expect(at(got, 2, 2)).toEqual([0, 0, 0, 0]);
	});

	test("a sharp shadow moves and does not tint the source pixel", () => {
		const src = solid(1, 1, red);
		const got = shadowOutline(
			src,
			1,
			1,
			{ shadow: { color: "#0000FF", opacity: 1, dx: 2, dy: 0, blur: 0 } },
			1,
			1,
			1,
		);
		expect(got.width).toBe(5);
		expect(got.height).toBe(1);
		expect(at(got, 2, 0)).toEqual([255, 0, 0, 255]);
		expect(at(got, 4, 0)).toEqual([0, 0, 255, 255]);
		expect(at(got, 0, 0)).toEqual([0, 0, 0, 0]);
		expect(at(got, 3, 0)).toEqual([0, 0, 0, 0]);
		expect(got.shadow).toMatchObject({ x: 2, y: 0, width: 1, height: 1 });
		expect(Array.from(got.shadow?.pixels ?? [])).toEqual([0, 0, 255, 255]);
	});

	test("opacity 0 leaves no shadow pixels and adds no pad", () => {
		const src = solid(1, 1, red);
		const before = src.slice();
		const got = shadowOutline(
			src,
			1,
			1,
			{ shadow: { color: "#00FF00", opacity: 0, dx: 3, dy: -2, blur: 5 } },
			1,
			1,
			1,
		);
		expect(got.pixels).toBe(src);
		expect(Array.from(src)).toEqual(Array.from(before));
		expect(got.width).toBe(1);
		expect(got.height).toBe(1);
		expect(got.shadow).toBeNull();
	});

	test("an outline whose colour alpha is 0 is not drawn", () => {
		const src = solid(1, 1, red);
		const got = shadowOutline(
			src,
			1,
			1,
			{ outline: { color: "#FFFFFF00", width: 4 } },
			1,
			1,
			1,
		);
		expect(got.pixels).toBe(src);
		expect(got.width).toBe(1);
		expect(got.height).toBe(1);
		expect(got.outline).toBeNull();
	});

	test("opacity 0.5 and an 8-digit colour premultiply the drop", () => {
		const src = solid(1, 1, red);
		const half = shadowOutline(
			src,
			1,
			1,
			{ shadow: { color: "#00FF00", opacity: 0.5, dx: 2, dy: 0, blur: 0 } },
			1,
			1,
			1,
		);
		const digit = shadowOutline(
			src,
			1,
			1,
			{ shadow: { color: "#00FF0080", opacity: 1, dx: 2, dy: 0, blur: 0 } },
			1,
			1,
			1,
		);
		expect(at(half, 2, 0)).toEqual([255, 0, 0, 255]);
		expect(at(half, 4, 0)).toEqual([0, 128, 0, 128]);
		expect(at(digit, 4, 0)).toEqual([0, 128, 0, 128]);
	});

	test("a blur of radius 1 is three box passes under the picture", () => {
		const src = solid(1, 1, red);
		// sigma = 1.5, so the box radius is 1 horizontally and 0 vertically.
		const got = shadowOutline(
			src,
			1,
			1,
			{ shadow: { color: "#FFFFFF", opacity: 1, dx: 0, dy: 0, blur: 3 } },
			1,
			1,
			1000,
		);
		expect(got.width).toBe(7);
		expect(got.height).toBe(1);
		const row = [9, 28, 57, 66, 57, 28, 9];
		for (let x = 0; x < row.length; x++) {
			const a = row[x] ?? 0;
			const want = x === 3 ? [255, 0, 0, 255] : [a, a, a, a];
			expect(at(got, x, 0)).toEqual(want);
		}
	});

	test("a project length is scaled into source pixels", () => {
		const src = solid(5, 1, red);
		// ox = round(3 * 2 * 5 / 6) = 5, oy = round(3 * 2 * 1 / 2) = 3.
		const got = shadowOutline(
			src,
			5,
			1,
			{ outline: { color: "#FFFFFF", width: 3 } },
			2,
			6,
			2,
		);
		expect(got.width).toBe(15);
		expect(got.height).toBe(7);
		expect(at(got, 5, 3)).toEqual([255, 0, 0, 255]);
		expect(at(got, 0, 3)).toEqual([255, 255, 255, 255]);
		expect(at(got, 0, 2)).toEqual([0, 0, 0, 0]);
	});

	test("a negative offset rounds half up", () => {
		const src = solid(1, 1, red);
		const got = shadowOutline(
			src,
			1,
			1,
			{ shadow: { color: "#00FF00", opacity: 1, dx: -1.6, dy: 0, blur: 0 } },
			1,
			1,
			1,
		);
		expect(got.width).toBe(5);
		expect(at(got, 2, 0)).toEqual([255, 0, 0, 255]);
		expect(at(got, 0, 0)).toEqual([0, 255, 0, 255]);
	});

	test("the outline is painted over the shadow and under the picture", () => {
		const src = solid(1, 1, red);
		const got = shadowOutline(
			src,
			1,
			1,
			{
				shadow: { color: "#00FF00", opacity: 1, dx: 2, dy: 0, blur: 0 },
				outline: { color: "#FFFFFF", width: 1 },
			},
			1,
			1,
			1,
		);
		expect(got.width).toBe(7);
		expect(got.height).toBe(3);
		expect(at(got, 3, 1)).toEqual([255, 0, 0, 255]);
		expect(at(got, 2, 1)).toEqual([255, 255, 255, 255]);
		expect(at(got, 4, 1)).toEqual([255, 255, 255, 255]);
		expect(at(got, 5, 1)).toEqual([0, 255, 0, 255]);
		expect(at(got, 0, 0)).toEqual([0, 0, 0, 0]);
	});
});

// PixiExecutor calls shadowOutline for the layer it draws. A test of the
// helper alone would stay green if the executor stopped painting it.
describe("preview shadow draw path", () => {
	test("the executor paints the plan shadow under the picture", () => {
		const source = executorSource;
		expect(source).toContain("applyShadowOutline(entry, layer, g)");
		const call = source.slice(source.indexOf("shadowOutline("));
		expect(call.slice(0, 300)).toContain("layer");
		expect(source).toContain("this.stage.scale.x");
	});
});
