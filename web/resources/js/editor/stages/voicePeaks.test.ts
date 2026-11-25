import { describe, expect, test } from "vitest";

import { bucketPeaks } from "./voicePeaks";

describe("the waveform columns of the studio voice screen", () => {
	test("one min and one max for each column of the samples", () => {
		const samples = new Float32Array([
			0.1, -0.5, 0.25, 0.75, -0.25, 0.0, 0.5, -1,
		]);
		expect(Array.from(bucketPeaks(samples, 4))).toEqual([
			-0.5, 0.10000000149011612, 0, 0.75, -0.25, 0, -1, 0.5,
		]);
	});

	test("an empty sound has flat columns, not a failure", () => {
		expect(Array.from(bucketPeaks(new Float32Array(), 3))).toEqual([
			0, 0, 0, 0, 0, 0,
		]);
	});
});
