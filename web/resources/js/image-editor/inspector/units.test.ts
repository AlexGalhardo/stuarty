import { describe, expect, test } from "vitest";

import { newLayerId, parseLength } from "./units";

describe("parseLength", () => {
	test("a bare number and px are pixels, rounded", () => {
		expect(parseLength("120", 1280)).toEqual({ value: 120 });
		expect(parseLength(" 120px ", 1280)).toEqual({ value: 120 });
		expect(parseLength("-5", 1280)).toEqual({ value: -5 });
		expect(parseLength("12.6", 1280)).toEqual({ value: 13 });
		expect(parseLength("12,4", 1280)).toEqual({ value: 12 });
	});

	test("a percent is a share of the canvas along the axis", () => {
		expect(parseLength("50%", 1280)).toEqual({ value: 640 });
		expect(parseLength("12.5 %", 720)).toEqual({ value: 90 });
	});

	test("a percent where the field has no axis, and text that is no length, are refused", () => {
		expect(parseLength("50%", 0)).toBeNull();
		expect(parseLength("", 1280)).toBeNull();
		expect(parseLength("12 s", 1280)).toBeNull();
		expect(parseLength("abc", 1280)).toBeNull();
	});
});

describe("newLayerId", () => {
	test("has the shape of a layer id", () => {
		for (let i = 0; i < 20; i++) {
			expect(newLayerId()).toMatch(/^lyr_[0-9a-hjkmnp-tv-z]{10}$/);
		}
	});
});
