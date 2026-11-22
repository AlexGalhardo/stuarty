import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";

import {
	clearPeaksCache,
	columnPeaks,
	isPeaksPending,
	loadPeaks,
	parsePeaks,
	watchPeaks,
} from "./waveform";

// The answer of the server while the job of a long sound runs (SDD-011 section 6.5).
const computing = (): Response =>
	Response.json(
		{
			code: "E_ASSET_NOT_READY",
			message: "the waveform of the asset is being computed in the background.",
			retryable: true,
			details: { reason: "peaks_pending", job_id: "job_1", progress: 0.5 },
		},
		{ status: 409 },
	);

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

	test("a 409 with peaks_pending means the server is computing them: no peaks yet, asked again later", async () => {
		vi.stubGlobal("fetch", vi.fn().mockResolvedValue(computing()));
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

	test("the 409 of an asset whose ingest failed is final: no wait fixes it", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn().mockResolvedValue(
				Response.json(
					{
						code: "E_ASSET_NOT_READY",
						message: "the asset is failed; its files are not ready.",
						retryable: true,
					},
					{ status: 409 },
				),
			),
		);
		expect(await loadPeaks("prj_c", "ast_3")).toBeNull();
		expect(isPeaksPending("prj_c", "ast_3")).toBe(false);
	});
});

describe("watchPeaks", () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});

	afterEach(() => {
		vi.useRealTimers();
		vi.unstubAllGlobals();
		clearPeaksCache();
	});

	test("asks again while the job runs and gives the peaks once", async () => {
		const fetcher = vi
			.fn()
			.mockImplementationOnce(() => Promise.resolve(computing()))
			.mockImplementationOnce(() => Promise.resolve(computing()))
			.mockImplementation(() =>
				Promise.resolve(new Response(file(), { status: 200 })),
			);
		vi.stubGlobal("fetch", fetcher);
		const results: unknown[] = [];

		watchPeaks("prj_w", "ast_1", 5000, (p) => results.push(p));
		await vi.advanceTimersByTimeAsync(0);
		expect(fetcher).toHaveBeenCalledTimes(1);
		expect(results).toEqual([]);

		await vi.advanceTimersByTimeAsync(5000);
		expect(fetcher).toHaveBeenCalledTimes(2);

		await vi.advanceTimersByTimeAsync(5000);
		expect(fetcher).toHaveBeenCalledTimes(3);
		expect(results).toHaveLength(1);
		expect(results[0]).not.toBeNull();

		await vi.advanceTimersByTimeAsync(60000);
		expect(fetcher).toHaveBeenCalledTimes(3);
	});

	test("stops when the sound leaves the page", async () => {
		const fetcher = vi
			.fn()
			.mockImplementation(() => Promise.resolve(computing()));
		vi.stubGlobal("fetch", fetcher);
		const results: unknown[] = [];

		const stop = watchPeaks("prj_w", "ast_2", 5000, (p) => results.push(p));
		await vi.advanceTimersByTimeAsync(5000);
		expect(fetcher).toHaveBeenCalledTimes(2);

		stop();
		await vi.advanceTimersByTimeAsync(60000);
		expect(fetcher).toHaveBeenCalledTimes(2);
		expect(results).toEqual([]);
	});

	test("stops on a final error and says so once", async () => {
		const fetcher = vi
			.fn()
			.mockImplementationOnce(() => Promise.resolve(computing()))
			.mockImplementation(() =>
				Promise.resolve(
					Response.json(
						{ code: "E_LIMIT_EXCEEDED", message: "too long" },
						{ status: 422 },
					),
				),
			);
		vi.stubGlobal("fetch", fetcher);
		const results: unknown[] = [];

		watchPeaks("prj_w", "ast_3", 5000, (p) => results.push(p));
		await vi.advanceTimersByTimeAsync(5000);
		expect(results).toEqual([null]);

		await vi.advanceTimersByTimeAsync(60000);
		expect(fetcher).toHaveBeenCalledTimes(2);
	});
});
