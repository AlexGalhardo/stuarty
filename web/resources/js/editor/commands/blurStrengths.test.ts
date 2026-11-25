import { describe, expect, test } from "vitest";

import { BLUR_STRENGTHS } from "./blurStrengths";
import { blurRegionOperations } from "./builders";

describe("the blur strengths of the core (SDD-002 C.90)", () => {
	test("the registry carries strong and medium, in this order, with their shares", () => {
		expect(BLUR_STRENGTHS.map((s) => s.id)).toEqual(["strong", "medium"]);
		expect(BLUR_STRENGTHS[0]?.share).toBe(0.04);
		expect(BLUR_STRENGTHS[1]?.share).toBeLessThan(0.04);
		expect(BLUR_STRENGTHS[1]?.share).toBeGreaterThan(0.015);
	});

	test("blur_region sends the strength by name and no radius", () => {
		const [op] = blurRegionOperations("itm_a", {
			x: 0.1,
			y: 0.2,
			width: 0.5,
			height: 0.1,
			strength: "medium",
		});
		expect(op?.args).toEqual({
			item_id: "itm_a",
			x: 0.1,
			y: 0.2,
			width: 0.5,
			height: 0.1,
			strength: "medium",
		});
	});
});
