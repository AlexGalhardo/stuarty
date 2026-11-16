import { describe, expect, it } from "vitest";
import {
	byteSpan,
	indexEnd,
	keyframeBefore,
	parseChunkIndex,
	sampleAt,
	sampleWindow,
} from "./chunkIndex";
import { makeIndex } from "./testIndex";

type Raw = Record<string, unknown>;

describe("parseChunkIndex", () => {
	it("accepts a valid index", () => {
		expect(parseChunkIndex(makeIndex()).ok).toBe(true);
	});

	const broken: [string, (i: Raw) => void][] = [
		[
			"an unknown version",
			(i) => {
				i.version = 2;
			},
		],
		[
			"an extra field",
			(i) => {
				i.extra = 1;
			},
		],
		[
			"a codec of another family",
			(i) => {
				i.codec = "avc1.42E01E";
			},
		],
		[
			"a zero coded width",
			(i) => {
				i.coded_width = 0;
			},
		],
		[
			"a coded height over 4096",
			(i) => {
				i.coded_height = 4097;
			},
		],
		[
			"a negative size",
			(i) => {
				(i.sizes as number[])[3] = -1;
			},
		],
		[
			"a zero offset",
			(i) => {
				(i.offsets as number[])[3] = 0;
			},
		],
		[
			"sizes of another length",
			(i) => {
				(i.sizes as number[]).pop();
			},
		],
		[
			"timestamps of another length",
			(i) => {
				(i.timestamps_us as number[]).pop();
			},
		],
		[
			"decreasing timestamps",
			(i) => {
				(i.timestamps_us as number[])[5] = 0;
			},
		],
		[
			"keyframes that do not start at 0",
			(i) => {
				(i.keyframes as number[])[0] = 1;
			},
		],
		[
			"keyframes that do not increase",
			(i) => {
				(i.keyframes as number[])[1] = 0;
			},
		],
		[
			"a keyframe past the last sample",
			(i) => {
				(i.keyframes as number[])[2] = 40;
			},
		],
		[
			"no samples",
			(i) => {
				i.offsets = [];
				i.sizes = [];
				i.timestamps_us = [];
			},
		],
		[
			"a non-integer",
			(i) => {
				(i.offsets as number[])[0] = 1.5;
			},
		],
	];
	for (const [name, mutate] of broken) {
		it(`refuses ${name}`, () => {
			const raw = structuredClone(makeIndex()) as unknown as Raw;
			mutate(raw);
			expect(parseChunkIndex(raw).ok).toBe(false);
		});
	}

	it("refuses what is not an object", () => {
		expect(parseChunkIndex(null).ok).toBe(false);
		expect(parseChunkIndex([]).ok).toBe(false);
		expect(parseChunkIndex("x").ok).toBe(false);
	});
});

describe("sampleAt", () => {
	const index = makeIndex();
	it("picks the sample that starts at the source time", () => {
		expect(sampleAt(index, 0)).toBe(0);
		expect(sampleAt(index, 10 / 30)).toBe(10);
		expect(sampleAt(index, 39 / 30)).toBe(39);
	});
	it("rounds to the nearest sample (half a sample of slack)", () => {
		expect(sampleAt(index, 10 / 30 + 0.012)).toBe(10);
		expect(sampleAt(index, 10 / 30 - 0.012)).toBe(10);
		expect(sampleAt(index, 10 / 30 + 0.02)).toBe(11);
	});
	it("clamps before the start and past the end", () => {
		expect(sampleAt(index, -1)).toBe(0);
		expect(sampleAt(index, 100)).toBe(39);
	});
	it("agrees with its own window for every sample", () => {
		for (let i = 0; i < 40; i++) {
			const { lo, hi } = sampleWindow(index, i);
			const ts = index.timestamps_us[i] as number;
			const mid =
				Number.isFinite(lo) && Number.isFinite(hi) ? (lo + hi) / 2 : ts;
			expect(sampleAt(index, mid / 1_000_000)).toBe(i);
		}
	});
	it("tiles the time axis with the windows", () => {
		expect(sampleWindow(index, 0).lo).toBe(-Infinity);
		expect(sampleWindow(index, 39).hi).toBe(Infinity);
		for (let i = 0; i < 39; i++) {
			expect(sampleWindow(index, i).hi).toBe(sampleWindow(index, i + 1).lo);
		}
	});
});

describe("keyframeBefore and byteSpan", () => {
	const index = makeIndex();
	it("finds the last key frame at or before a sample", () => {
		expect(keyframeBefore(index, 0)).toBe(0);
		expect(keyframeBefore(index, 14)).toBe(0);
		expect(keyframeBefore(index, 15)).toBe(15);
		expect(keyframeBefore(index, 39)).toBe(30);
	});
	it("spans the bytes of a run of samples", () => {
		const { start, end } = byteSpan(index, 2, 4);
		expect(start).toBe(index.offsets[2]);
		expect(end).toBe((index.offsets[4] as number) + (index.sizes[4] as number));
	});
	it("takes the min and the max when the samples are not in order", () => {
		const shuffled = structuredClone(index);
		[shuffled.offsets[2], shuffled.offsets[3]] = [
			shuffled.offsets[3] as number,
			shuffled.offsets[2] as number,
		];
		const { start, end } = byteSpan(shuffled, 2, 3);
		expect(start).toBe(
			Math.min(index.offsets[2] as number, index.offsets[3] as number),
		);
		expect(end).toBeGreaterThan(start);
	});
	it("reports the end of the last byte read", () => {
		expect(indexEnd(index)).toBe(
			(index.offsets[39] as number) + (index.sizes[39] as number),
		);
	});
});
