import { describe, expect, it } from "vitest";

import { blendChannel, div255, pixiBlendMode } from "./blend";

// The formulas of SDD-004 section 3.8, opaque source (a = 255) and a source
// at opacity 128 (premultiplied c = div255(c0 * 128), a = 128).
describe("blend reference", () => {
	it("div255 is exact over its range", () => {
		for (let x = 0; x <= 65535; x++)
			expect(div255(x)).toBe(Math.round(x / 255));
	});

	it("matches the closed forms for an opaque source", () => {
		for (const c of [0, 1, 77, 128, 200, 255]) {
			for (const d of [0, 1, 127, 128, 130, 255]) {
				expect(blendChannel("multiply", c, 255, d)).toBe(div255(d * c));
				expect(blendChannel("screen", c, 255, d)).toBe(c + d - div255(c * d));
				expect(blendChannel("add", c, 255, d)).toBe(Math.min(c + d, 255));
				expect(blendChannel("overlay", c, 255, d)).toBe(
					d <= 127 ? div255(2 * c * d) : c + div255((2 * d - 255) * (255 - c)),
				);
				expect(blendChannel("normal", c, 255, d)).toBe(c);
			}
		}
	});

	it("has the identities of each mode", () => {
		expect(blendChannel("multiply", 255, 255, 90)).toBe(90);
		expect(blendChannel("screen", 0, 255, 90)).toBe(90);
		expect(blendChannel("add", 200, 255, 90)).toBe(255);
		expect(blendChannel("overlay", 128, 255, 90)).toBe(div255(2 * 128 * 90));
	});

	it("draws nothing for a transparent source and keeps half opacity between", () => {
		for (const m of ["multiply", "screen", "overlay", "add"] as const) {
			expect(blendChannel(m, 0, 0, 100)).toBe(100);
		}
		// multiply of white at opacity 128 leaves the destination unchanged
		expect(blendChannel("multiply", 128, 128, 100)).toBe(100);
		// screen of black at opacity 128 leaves the destination unchanged
		expect(blendChannel("screen", 0, 128, 100)).toBe(100);
	});

	it("maps an absent blend to normal", () => {
		expect(pixiBlendMode(undefined)).toBe("normal");
		expect(pixiBlendMode("overlay")).toBe("overlay");
	});
});
