import { describe, expect, test } from "vitest";

import { dropTarget, type HeaderRect } from "./reorder";

const rects: HeaderRect[] = [
	{ id: "a", top: 0, bottom: 64 },
	{ id: "b", top: 64, bottom: 128 },
	{ id: "c", top: 128, bottom: 176 },
];

describe("dropTarget", () => {
	test("the upper half of a row means above it, the lower half below it", () => {
		expect(dropTarget(rects, "c", 70)?.placement).toEqual({ above: "b" });
		expect(dropTarget(rects, "a", 100)?.placement).toEqual({ below: "b" });
	});

	test("past the first or last row means the top or the bottom", () => {
		expect(dropTarget(rects, "c", -5)?.placement).toEqual({ position: "top" });
		expect(dropTarget(rects, "a", 500)?.placement).toEqual({
			position: "bottom",
		});
	});

	test("the line sits on the edge it names", () => {
		expect(dropTarget(rects, "c", 70)?.lineY).toBe(64);
		expect(dropTarget(rects, "a", 100)?.lineY).toBe(128);
	});

	test("the place the track already has is not a drop", () => {
		expect(dropTarget(rects, "b", 90)).toBeNull();
		expect(dropTarget(rects, "a", 10)).toBeNull();
		expect(dropTarget(rects, "b", 10)).toEqual(
			expect.objectContaining({ placement: { above: "a" } }),
		);
		expect(dropTarget(rects, "a", 100)).not.toBeNull();
		expect(dropTarget(rects, "b", 70)).toBeNull();
		expect(dropTarget(rects, "b", 120)).toBeNull();
		expect(dropTarget(rects, "a", -1)).toBeNull();
		expect(dropTarget(rects, "c", 999)).toBeNull();
	});

	test("an unknown track has no drop", () => {
		expect(dropTarget(rects, "x", 10)).toBeNull();
	});
});
