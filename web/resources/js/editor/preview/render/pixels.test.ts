import { describe, expect, it } from "vitest";

import {
	cutRows,
	decodeVideoPlanes,
	evenAround,
	videoPlaneLayout,
	whole,
} from "./pixels";

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

	it("decodes limited-range BT.709 I420 and NV12 planes for a cropped region", () => {
		// 4 x 2 frame: left half blue (Y=32, U=240, V=118), right half green (Y=173, U=42, V=26).
		const yPlane = new Uint8Array([32, 32, 173, 173, 32, 32, 173, 173]);
		const uPlane = new Uint8Array([240, 42]);
		const vPlane = new Uint8Array([118, 26]);
		const i420 = new Uint8Array(yPlane.length + uPlane.length + vPlane.length);
		i420.set(yPlane, 0);
		i420.set(uPlane, 8);
		i420.set(vPlane, 10);
		const rgba = decodeVideoPlanes(
			i420,
			[
				{ offset: 0, stride: 4 },
				{ offset: 8, stride: 2 },
				{ offset: 10, stride: 2 },
			],
			"I420",
			{ matrix: "bt709", fullRange: false },
			{ x: 1, y: 0, w: 2, h: 1 },
		);
		// Pixel 0 is blue (high B, low R/G); pixel 1 is green (high G, low R/B).
		expect(rgba[0]).toBeLessThan(15);
		expect(rgba[1]).toBeLessThan(15);
		expect(rgba[2]).toBeGreaterThan(240);
		expect(rgba[3]).toBe(255);
		expect(rgba[4]).toBeLessThan(15);
		expect(rgba[5]).toBeGreaterThan(240);
		expect(rgba[6]).toBeLessThan(15);
		expect(rgba[7]).toBe(255);

		const nv12 = new Uint8Array(yPlane.length + 4);
		nv12.set(yPlane, 0);
		nv12.set([240, 118, 42, 26], 8);
		const nv12Rgba = decodeVideoPlanes(
			nv12,
			[
				{ offset: 0, stride: 4 },
				{ offset: 8, stride: 4 },
			],
			"NV12",
			{ matrix: "bt709", fullRange: false },
			{ x: 1, y: 0, w: 2, h: 1 },
		);
		expect([...nv12Rgba]).toEqual([...rgba]);
	});

	it("keeps chroma rows and plane offsets distinct for odd image dimensions", () => {
		expect(videoPlaneLayout("NV12", 5, 3)).toEqual([
			{ offset: 0, stride: 5 },
			{ offset: 15, stride: 6 },
		]);
		expect(videoPlaneLayout("I420A", 5, 3)).toEqual([
			{ offset: 0, stride: 5 },
			{ offset: 15, stride: 3 },
			{ offset: 21, stride: 3 },
			{ offset: 27, stride: 5 },
		]);
	});
});
