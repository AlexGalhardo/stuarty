import { describe, expect, test } from "vitest";

import type { Job } from "../../editor/api/types";
import { exportLabel } from "./exportLabel";

const job = (made: Record<string, unknown>): Job =>
	({
		job_id: "job_se4qrp76xb",
		kind: "export",
		state: "succeeded",
		result: { export: made },
	}) as unknown as Job;

describe("the name of an export in the list a person picks from", () => {
	test("says the preset, the size and the date, not the id of the job", () => {
		const label = exportLabel(
			job({ preset: "youtube-shorts", width: 1080, height: 1920 }),
			"Nov 20, 10:00",
		);
		expect(label).toBe("youtube-shorts · 1080×1920 · Nov 20, 10:00");
		expect(label).not.toContain("job_");
	});

	test("falls back to the format, and leaves out a size it does not have", () => {
		expect(exportLabel(job({ format: "png", width: 0, height: 0 }), "d")).toBe(
			"png · d",
		);
	});
});
