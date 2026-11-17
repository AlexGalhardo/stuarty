import { describe, expect, test } from "vitest";

import type { PlanLayer, PlanMask } from "../api/types";
import { cropAt, maskAt } from "./planValue";

const mask = (x: number): PlanMask => ({
	shape: "rect",
	x,
	y: 0.1,
	width: 0.5,
	height: 0.5,
	feather: 0,
	invert: false,
});

function media(
	extra: Partial<Extract<PlanLayer, { kind: "media" }>>,
): PlanLayer {
	return {
		kind: "media",
		first: 10,
		crop: [0, 0, 0, 0],
		...extra,
	} as unknown as PlanLayer;
}

describe("per-frame mask and crop of a layer", () => {
	test("the constant value is used without a series", () => {
		expect(maskAt(media({ mask: mask(0.2) }), 12)).toEqual(mask(0.2));
		expect(maskAt(media({}), 12)).toBeNull();
		expect(cropAt(media({ crop: [0.1, 0, 0, 0] }), 12)).toEqual([0.1, 0, 0, 0]);
	});

	test("a series is indexed from the first frame and clamped", () => {
		const layer = media({
			masks: [mask(0.1), null, mask(0.3)],
			crops: [
				[0, 0, 0, 0],
				[0.1, 0, 0, 0],
			],
		});
		expect(maskAt(layer, 10)).toEqual(mask(0.1));
		expect(maskAt(layer, 11)).toBeNull();
		expect(maskAt(layer, 99)).toEqual(mask(0.3));
		expect(maskAt(layer, 0)).toEqual(mask(0.1));
		expect(cropAt(layer, 11)).toEqual([0.1, 0, 0, 0]);
		expect(cropAt(layer, 50)).toEqual([0.1, 0, 0, 0]);
	});
});
