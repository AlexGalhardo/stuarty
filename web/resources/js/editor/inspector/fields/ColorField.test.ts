import { describe, expect, test } from "vitest";

import { joinHex, splitHex } from "./ColorField";

describe("colour hex", () => {
	test("#RRGGBB when opaque, #RRGGBBAA otherwise", () => {
		expect(joinHex("#ff8800", 255)).toBe("#FF8800");
		expect(joinHex("#ff8800", 128)).toBe("#FF880080");
		expect(joinHex("#ff8800", 0)).toBe("#FF880000");
	});

	test("splitHex reads both forms and rejects the rest", () => {
		expect(splitHex("#ff8800")).toEqual({ rgb: "#FF8800", alpha: 255 });
		expect(splitHex(" #FF880080 ")).toEqual({ rgb: "#FF8800", alpha: 128 });
		for (const bad of ["", "ff8800", "#fff", "#ff88", "#GG0000", "#ff880080ff"])
			expect(splitHex(bad)).toBeNull();
	});
});
