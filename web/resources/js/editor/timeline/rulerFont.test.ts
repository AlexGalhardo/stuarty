import { describe, expect, test } from "vitest";

import { rulerFont } from "./geometry";

describe("rulerFont", () => {
	test("draws the labels at 12 px in the family of the theme, never through var()", () => {
		const font = rulerFont(' "Inter Variable", ui-sans-serif, sans-serif');
		expect(font).toBe('12px "Inter Variable", ui-sans-serif, sans-serif');
		expect(font).not.toContain("var(");
	});

	test("falls back to sans-serif when the theme has no family", () => {
		expect(rulerFont("  ")).toBe("12px sans-serif");
	});
});
