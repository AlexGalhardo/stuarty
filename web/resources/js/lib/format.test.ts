import { describe, expect, test } from "vitest";

import { formatAgo } from "./format";

const now = new Date("2026-11-24T12:00:00Z");

describe("formatAgo", () => {
	test("says the largest unit that fits", () => {
		expect(formatAgo("2026-11-24T11:59:30Z", "en", now)).toMatch(/0|now/);
		expect(formatAgo("2026-11-24T11:56:00Z", "en", now)).toMatch(/4/);
		expect(formatAgo("2026-11-24T09:00:00Z", "en", now)).toMatch(/3/);
		expect(formatAgo("2026-11-21T12:00:00Z", "en", now)).toMatch(/3/);
	});

	test("a moment in the future reads as now, and a bad date is shown as it came", () => {
		expect(formatAgo("2026-11-24T12:05:00Z", "en", now)).toMatch(/0|now/);
		expect(formatAgo("soon", "en", now)).toBe("soon");
	});
});
