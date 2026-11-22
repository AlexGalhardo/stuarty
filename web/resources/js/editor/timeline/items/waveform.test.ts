import { afterEach, describe, expect, test, vi } from "vitest";

import {
	clearPeaksCache,
	columnPeaks,
	isPeaksPending,
	loadPeaks,
	parsePeaks,
} from "./waveform";

// A peaks file of version 1 with two levels (4 and 16 samples per peak) over 64
// samples at 48 kHz: the first half is quiet (+-1000), the second loud (+-20000).
function file(): ArrayBuffer {
	const levels = [
		{ spp: 4, count: 16 },
		{ spp: 16, count: 4 },
	];
	const table = 24 + levels.length * 16;
	const buffer = new ArrayBuffer(table + (16 + 4) * 4);
	const view = new DataView(buffer);
	view.setUint32(0, 0x4b50_4b56, true);
	view.setUint16(4, 1, true);
	view.setUint16(6, levels.length, true);
	view.setUint32(8, 48000, true);
	view.setBigUint64(16, 64n, true);
	let offset = table;
	levels.forEach((level, i) => {
		const at = 24 + i * 16;
		view.setUint32(at, level.spp, true);
		view.setUint32(at + 4, level.count, true);
		view.setBigUint64(at + 8, BigInt(offset), true);
		for (let p = 0; p < level.count; p++) {
			const loud = p >= level.count / 2;
			view.setInt16(offset + p * 4, loud ? -20000 : -1000, true);
			view.setInt16(offset + p * 4 + 2, loud ? 20000 : 1000, true);
		}
		offset += level.count * 4;
	});
	return buffer;
}

const fps = { num: 48000, den: 1 }; // one project frame is one sample
const base = {
	sourceIn: 0,
	speed: { num: 1, den: 1 },
	fps,
	pxPerFrame: 1,
	firstColumn: 0,
	columns: 64,
};

describe("waveform", () => {
	test("reads the table of levels", () => {
		const peaks = parsePeaks(file());
		expect(peaks?.sampleCount).toBe(64);
		expect(peaks?.levels.map((level) => level.samplesPerPeak)).toEqual([4, 16]);
	});

	test("refuses a file that is not a version 1 peaks file", () => {
		expect(parsePeaks(new ArrayBuffer(8))).toBeNull();
		const bad = file();
		new DataView(bad).setUint16(4, 2, true);
		expect(parsePeaks(bad)).toBeNull();
	});

	test("a column is the min and max of the samples it covers, outside the file 0", () => {
		const peaks = parsePeaks(file());
		if (peaks === null) throw new Error("no peaks");
		const bars = columnPeaks(peaks, { ...base, columns: 80 });
		expect(bars[0]).toBeCloseTo(-1000 / 32768);
		expect(bars[1]).toBeCloseTo(1000 / 32768);
		expect(bars[2 * 40 + 1]).toBeCloseTo(20000 / 32768);
		expect(bars[2 * 70]).toBe(0);
		expect(bars[2 * 70 + 1]).toBe(0);
	});

	test("a clip that starts later reads later samples", () => {
		const peaks = parsePeaks(file());
		if (peaks === null) throw new Error("no peaks");
		const bars = columnPeaks(peaks, { ...base, sourceIn: 32, columns: 8 });
		expect(bars[1]).toBeCloseTo(20000 / 32768);
	});

	test("zoomed out, a column takes the coarser level and still sees the peak", () => {
		const peaks = parsePeaks(file());
		if (peaks === null) throw new Error("no peaks");
		const bars = columnPeaks(peaks, {
			...base,
			pxPerFrame: 1 / 32,
			columns: 2,
		});
		expect(bars[1]).toBeCloseTo(1000 / 32768);
		expect(bars[3]).toBeCloseTo(20000 / 32768);
	});
});

describe("loadPeaks of a long sound", () => {
	afterEach(() => {
		vi.unstubAllGlobals();
		clearPeaksCache();
	});

	test("a 409 means the server is computing them: no peaks yet, asked again later", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(new Response("{}", { status: 409 })),
		);
		expect(await loadPeaks("prj_a", "ast_1")).toBeNull();
		expect(isPeaksPending("prj_a", "ast_1")).toBe(true);

		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(new Response(file(), { status: 200 })),
		);
		expect(await loadPeaks("prj_a", "ast_1")).not.toBeNull();
		expect(isPeaksPending("prj_a", "ast_1")).toBe(false);
	});

	test("any other failure is final", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(new Response("{}", { status: 422 })),
		);
		expect(await loadPeaks("prj_b", "ast_2")).toBeNull();
		expect(isPeaksPending("prj_b", "ast_2")).toBe(false);
	});
});
