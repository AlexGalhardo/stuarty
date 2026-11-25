import { describe, expect, test } from "vitest";

import { revealsFailure } from "./Failure";

describe("revealsFailure", () => {
	test("brings a note into view inside a dialog", () => {
		const note = {
			closest: (selector: string) =>
				selector === '[role="dialog"]' ? {} : null,
		};
		expect(revealsFailure(note)).toBe(true);
	});

	test("leaves the page where it is for a note on a page (VV-431)", () => {
		expect(revealsFailure({ closest: () => null })).toBe(false);
		expect(revealsFailure(null)).toBe(false);
	});
});
