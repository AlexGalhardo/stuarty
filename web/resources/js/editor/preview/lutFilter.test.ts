import { describe, expect, it } from "vitest";

import { parseLut } from "../api/lut";
import { lutAtlas, lutMix, MAX_LUT_SIZE } from "./lutFilter";

describe("lutMix", () => {
	it("is the intensity in thousandths, held to 0..1000", () => {
		expect(lutMix(0)).toBe(0);
		expect(lutMix(0.5)).toBe(500);
		expect(lutMix(0.8)).toBe(800);
		expect(lutMix(1)).toBe(1000);
		expect(lutMix(3)).toBe(1000);
		expect(lutMix(-1)).toBe(0);
		expect(lutMix(0.0004)).toBe(0);
		expect(lutMix(Number.NaN)).toBe(0);
	});
});

describe("lutAtlas", () => {
	it("puts the high and low bytes of entry (r, g, b) at texels (2r, g + n*b) and (2r + 1, g + n*b)", () => {
		const n = 2;
		const buffer = new ArrayBuffer(7 + 6 * n ** 3);
		const view = new DataView(buffer);
		"VLUT".split("").forEach((c, i) => {
			view.setUint8(i, c.charCodeAt(0));
		});
		view.setUint8(4, 1);
		view.setUint16(5, n, true);
		for (let i = 0; i < 3 * n ** 3; i++) {
			view.setUint16(7 + 2 * i, 0x1234 + i * 0x0101, true);
		}
		const table = parseLut(buffer);
		expect(table).not.toBeNull();
		if (table === null) return;
		const width = 2 * MAX_LUT_SIZE;
		const atlas = new Uint8Array(width * MAX_LUT_SIZE * MAX_LUT_SIZE * 4);
		lutAtlas(table, atlas);
		for (let b = 0; b < n; b++) {
			for (let g = 0; g < n; g++) {
				for (let r = 0; r < n; r++) {
					const entry = 3 * (r + n * (g + n * b));
					const row = g + n * b;
					const hi = (2 * r + row * width) * 4;
					for (let c = 0; c < 3; c++) {
						const v = table.entries[entry + c];
						expect(atlas[hi + c]).toBe(v >> 8);
						expect(atlas[hi + 4 + c]).toBe(v & 255);
					}
					expect(atlas[hi + 3]).toBe(255);
				}
			}
		}
	});
});
