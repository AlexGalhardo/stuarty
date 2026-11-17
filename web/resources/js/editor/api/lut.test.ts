import { describe, expect, it } from "vitest";

import { parseLut } from "./lut";

function table(size: number, edit?: (v: DataView) => void): ArrayBuffer {
	const buffer = new ArrayBuffer(7 + 6 * size ** 3);
	const v = new DataView(buffer);
	"VLUT".split("").forEach((c, i) => {
		v.setUint8(i, c.charCodeAt(0));
	});
	v.setUint8(4, 1);
	v.setUint16(5, size, true);
	for (let i = 0; i < 3 * size ** 3; i++) v.setUint16(7 + 2 * i, i, true);
	edit?.(v);
	return buffer;
}

describe("parseLut", () => {
	it("reads a good table, red fastest", () => {
		const t = parseLut(table(2));
		expect(t?.size).toBe(2);
		expect(t?.entries.length).toBe(24);
		expect(Array.from(t?.entries.slice(3, 6) ?? [])).toEqual([3, 4, 5]);
	});

	it("refuses a bad magic", () => {
		expect(parseLut(table(2, (v) => v.setUint8(0, 0x58)))).toBeNull();
	});

	it("refuses another version", () => {
		expect(parseLut(table(2, (v) => v.setUint8(4, 2)))).toBeNull();
	});

	it("refuses a size outside 2 to 65", () => {
		expect(parseLut(table(2, (v) => v.setUint16(5, 1, true)))).toBeNull();
		expect(parseLut(table(2, (v) => v.setUint16(5, 66, true)))).toBeNull();
	});

	it("refuses a size that does not fit the length", () => {
		expect(parseLut(table(2, (v) => v.setUint16(5, 3, true)))).toBeNull();
	});

	it("refuses a truncated or extended table", () => {
		expect(parseLut(table(3).slice(0, 100))).toBeNull();
		expect(parseLut(new ArrayBuffer(3))).toBeNull();
		const long = new Uint8Array(table(2).byteLength + 1);
		long.set(new Uint8Array(table(2)));
		expect(parseLut(long.buffer)).toBeNull();
	});
});
