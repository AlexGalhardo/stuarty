import { describe, expect, test } from "vitest";

import {
	AUDIO_PRESETS,
	AUDIO_PROCESSORS,
	MAX_PROCESSORS,
} from "./processorTable";

// The table is the one the core publishes in the registry: a missing or short one is a mismatch between the editor and
// the core, and fails here.
describe("the audio processor table of the registry", () => {
	test("has the six kinds, each with ranges", () => {
		expect(AUDIO_PROCESSORS.map((k) => k.kind)).toEqual([
			"eq",
			"compressor",
			"limiter",
			"gate",
			"deesser",
			"noise_reduction",
		]);
		for (const kind of AUDIO_PROCESSORS)
			for (const p of kind.params) {
				expect(p.min).toBeLessThanOrEqual(p.max);
				expect(p.default).toBeGreaterThanOrEqual(p.min);
				expect(p.default).toBeLessThanOrEqual(p.max);
			}
		const eq = AUDIO_PROCESSORS.find((k) => k.kind === "eq");
		expect(eq?.params).toHaveLength(2 + 6 * 3);
	});

	test("lists the four presets and the longest chain", () => {
		expect(AUDIO_PRESETS).toEqual([
			"voice",
			"voice_with_music",
			"podcast",
			"clean_noisy_room",
		]);
		expect(MAX_PROCESSORS).toBe(12);
	});
});
