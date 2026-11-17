import { describe, expect, test } from "vitest";

import type { PlanLayer, PlanMask } from "../api/types";
import executorSource from "./PixiExecutor.ts?raw";
import { cropAt, maskAt } from "./planValue";

const mask = (x: number): PlanMask => ({
	shape: "rect",
	x,
	y: 0,
	width: 0.5,
	height: 0.5,
});

function media(
	extra: Partial<Extract<PlanLayer, { kind: "media" }>>,
): PlanLayer {
	return {
		kind: "media",
		first: 10,
		crop: [0, 0, 8, 8],
		...extra,
	} as unknown as PlanLayer;
}

// PixiExecutor calls maskAt and cropAt for the frame it draws. A test of the
// helpers alone would stay green if the executor went back to layer.mask and
// layer.crop, so this also reads the executor.
describe("preview frame mask and crop", () => {
	test("the executor draws the series entry for that frame", () => {
		const source = executorSource;
		expect(source).toContain("maskAt(layer, frame)");
		expect(source).toContain("cropAt(layer, frame)");
		expect(source).not.toContain("layer.mask");
		expect(source).not.toContain("layer.crop");

		const layer = media({
			mask: mask(0.2),
			masks: [mask(0.1), null, mask(0.4)],
			crops: [
				[0, 0, 8, 8],
				[2, 3, 4, 5],
				[9, 9, 1, 1],
			],
		});
		expect(maskAt(layer, 11)).toBeNull();
		expect(maskAt(layer, 12)).toEqual(mask(0.4));
		expect(cropAt(layer, 11)).toEqual([2, 3, 4, 5]);
		expect(cropAt(media({ crop: [1, 2, 3, 4] }), 12)).toEqual([1, 2, 3, 4]);
	});
});
