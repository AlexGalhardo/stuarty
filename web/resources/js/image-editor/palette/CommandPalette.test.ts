import { describe, expect, test } from "vitest";

import { isPaletteKey } from "./CommandPalette";

const key = (over: Partial<Parameters<typeof isPaletteKey>[0]>) => ({
	key: "k",
	ctrlKey: false,
	metaKey: false,
	altKey: false,
	shiftKey: false,
	...over,
});

describe("isPaletteKey", () => {
	test("Ctrl K and Cmd K open the palette, in either case", () => {
		expect(isPaletteKey(key({ ctrlKey: true }))).toBe(true);
		expect(isPaletteKey(key({ metaKey: true, key: "K" }))).toBe(true);
	});

	test("K alone, or with Alt or Shift, does not", () => {
		expect(isPaletteKey(key({}))).toBe(false);
		expect(isPaletteKey(key({ ctrlKey: true, altKey: true }))).toBe(false);
		expect(isPaletteKey(key({ ctrlKey: true, shiftKey: true }))).toBe(false);
		expect(isPaletteKey(key({ ctrlKey: true, key: "j" }))).toBe(false);
	});
});
