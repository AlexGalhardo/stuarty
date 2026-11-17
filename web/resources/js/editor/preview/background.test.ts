import { describe, expect, test } from "vitest";

import { uncoveredPixel } from "./background";

describe("preview background", () => {
	test("an uncovered pixel stays transparent", () => {
		const pixel = uncoveredPixel({
			background: "#000000",
			transparent: true,
		});
		expect(pixel).toEqual([0, 0, 0, 0]);
		expect(pixel[3]).toBe(0);
	});

	test("the document word transparent leaves the pixel clear", () => {
		expect(uncoveredPixel({ background: "transparent" })).toEqual([0, 0, 0, 0]);
	});

	test("a colour background is opaque", () => {
		expect(uncoveredPixel({ background: "#112233" })).toEqual([
			0x11, 0x22, 0x33, 255,
		]);
		expect(uncoveredPixel({ background: "#000000" })).toEqual([0, 0, 0, 255]);
	});
});
