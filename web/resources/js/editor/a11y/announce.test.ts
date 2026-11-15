import { beforeEach, describe, expect, test } from "vitest";

import { announce, resetAnnouncements } from "./announce";

beforeEach(resetAnnouncements);

describe("announce", () => {
	test("a key announced again inside its interval is dropped", () => {
		let at = 0;
		const now = (): number => at;
		const sent: boolean[] = [];
		// Five remote edits in one second: one announcement (SDD-012 section 18.1).
		for (const t of [0, 200, 400, 600, 800]) {
			at = t;
			sent.push(announce("remote", undefined, { minIntervalMs: 2000, now }));
		}
		expect(sent).toEqual([true, false, false, false, false]);
		at = 2000;
		expect(announce("remote", undefined, { minIntervalMs: 2000, now })).toBe(
			true,
		);
	});

	test("without an interval every announcement goes out", () => {
		expect(announce("split", { count: 2 })).toBe(true);
		expect(announce("split", { count: 2 })).toBe(true);
	});
});
