import { describe, expect, it } from "vitest";

import { cutRows, evenAround, whole } from "./pixels";

describe("pixels", () => {
	it("rounds a crop to whole pixels and keeps it on the frame", () => {
		expect(whole({ x: 7.2, y: -0.4, w: 107.6, h: 0.2 })).toEqual({
			x: 7,
			y: 0,
			w: 108,
			h: 1,
		});
	});

	it("widens a rectangle to even coordinates inside the frame", () => {
		expect(evenAround({ x: 45, y: 32, w: 90, h: 192 }, 180, 320)).toEqual({
			x: 44,
			y: 32,
			w: 92,
			h: 192,
		});
		expect(evenAround({ x: 1, y: 1, w: 178, h: 318 }, 180, 320)).toEqual({
			x: 0,
			y: 0,
			w: 180,
			h: 320,
		});
		expect(evenAround({ x: 4, y: 6, w: 3, h: 3 }, 8, 8)).toEqual({
			x: 4,
			y: 6,
			w: 4,
			h: 2,
		});
	});

	it("cuts the crop out of the even rectangle", () => {
		// 4 x 2 pixels whose red channel is the column.
		const outer = { x: 2, y: 0, w: 4, h: 2 };
		const px = new Uint8Array(4 * 2 * 4);
		for (let row = 0; row < 2; row++) {
			for (let col = 0; col < 4; col++)
				px[(row * 4 + col) * 4] = 10 * col + row;
		}
		const cut = cutRows(px, outer, { x: 3, y: 1, w: 2, h: 1 });
		expect([...cut]).toEqual([11, 0, 0, 0, 21, 0, 0, 0]);
	});
});
