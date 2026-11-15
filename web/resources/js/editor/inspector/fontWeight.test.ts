import { describe, expect, test } from "vitest";

import { fontFamilyChanges } from "./fontWeight";

describe("fontFamilyChanges", () => {
	test("keeps only the family when the weight exists", () => {
		expect(fontFamilyChanges("Inter", [400, 700], 400)).toEqual({
			font_family: "Inter",
		});
	});

	test("uses the core default 700 when the style names no weight", () => {
		expect(fontFamilyChanges("Inter", [400, 700], undefined)).toEqual({
			font_family: "Inter",
		});
		expect(fontFamilyChanges("Anton", [400], undefined)).toEqual({
			font_family: "Anton",
			font_weight: 400,
		});
	});

	test("picks the nearest weight, the heavier on a tie", () => {
		expect(fontFamilyChanges("X", [300, 500], 400)).toEqual({
			font_family: "X",
			font_weight: 500,
		});
		expect(fontFamilyChanges("X", [100, 300], 900)).toEqual({
			font_family: "X",
			font_weight: 300,
		});
	});

	test("sends only the family when the family lists no weights", () => {
		expect(fontFamilyChanges("X", [], 700)).toEqual({ font_family: "X" });
	});
});
