import { describe, expect, test } from "vitest";

import type { PlanMediaLayer } from "../api/types";
import {
	angleCS,
	dipOpacity,
	effectPasses,
	FX,
	grainAmplitude,
	hexColour,
	SIN_TABLE,
	screenDirection,
	whipLength,
} from "./effects";

const layer = (extra: Partial<PlanMediaLayer>): PlanMediaLayer => ({
	kind: "media",
	item: "itm_a",
	first: 10,
	last: 40,
	cx: 540,
	cy: 960,
	w: 1080,
	h: 1920,
	rotation: 0,
	scale: 1,
	opacity: 1,
	asset: "ast_a",
	asset_kind: "video",
	source_seconds: [0],
	crop: [0, 0, 1, 1],
	color_matrix: null,
	...extra,
});

const ctx = {
	scale: 1,
	rotation: 0,
	frameWidth: 1080,
	frameHeight: 1920,
};

describe("the sine table of SDD-004 section 3.13", () => {
	test("has 91 values, round(1024 sin d)", () => {
		expect(SIN_TABLE).toHaveLength(91);
		for (let d = 0; d <= 90; d++) {
			expect(SIN_TABLE[d]).toBe(
				Math.round(1024 * Math.sin((d * Math.PI) / 180)),
			);
		}
	});

	test("an angle gives the cosine and the sine of its quadrant", () => {
		expect(angleCS(0)).toEqual({ c: 1024, s: 0 });
		expect(angleCS(90)).toEqual({ c: 0, s: 1024 });
		expect(angleCS(-90)).toEqual({ c: 0, s: -1024 });
		expect(angleCS(180)).toEqual({ c: -1024, s: 0 });
		expect(angleCS(30)).toEqual({ c: 887, s: 512 });
		expect(angleCS(150)).toEqual({ c: -887, s: 512 });
		expect(angleCS(-150)).toEqual({ c: -887, s: -512 });
	});

	test("the direction turns with the layer", () => {
		const [x, y] = screenDirection(0, 90);
		expect(x).toBeCloseTo(0);
		expect(y).toBeCloseTo(1);
	});
});

describe("the integers of the transitions and the grain", () => {
	test("the whip blur is at most a sixteenth of the frame", () => {
		expect(whipLength(1, 1080)).toBe(67);
		expect(whipLength(65535, 1080)).toBe(0);
		expect(whipLength(32768, 1080)).toBe(34);
	});

	test("the dip keeps all of the layer at the end of the progress", () => {
		expect(dipOpacity(65535)).toBe(255);
		expect(dipOpacity(1)).toBe(0);
		expect(dipOpacity(32768)).toBe(128);
	});

	test("the grain amplitude is amount x 64 in thousandths", () => {
		expect(grainAmplitude(0.3)).toBe(19);
		expect(grainAmplitude(1)).toBe(64);
		expect(grainAmplitude(0)).toBe(0);
	});

	test("a dip colour is read from the hex", () => {
		expect(hexColour("#FF8000")).toEqual([1, 128 / 255, 0]);
		expect(hexColour(undefined)).toEqual([0, 0, 0]);
	});
});

describe("the passes of a layer", () => {
	test("keep the order of the effects, then the whip or the dip before the mask", () => {
		const passes = effectPasses(
			layer({
				effects: [
					{ kind: "vignette", amount: 0.5, softness: 0.5 },
					{ kind: "motion_blur", amount: 0.5, angle: 0 },
				],
				transition: [{ kind: "dip", edge: "in", p: 32768, color: "#FF0000" }],
			}),
			10,
			ctx,
		);
		expect(passes.before.map((p) => p.kind)).toEqual([
			FX.vignette,
			FX.motion_blur,
			FX.dip,
		]);
		expect(passes.after).toEqual([]);
	});

	test("a wipe runs after the mask", () => {
		const passes = effectPasses(
			layer({
				transition: [
					null,
					{ kind: "wipe", edge: "out", p: 16384, direction: "up" },
				],
			}),
			11,
			ctx,
		);
		expect(passes.before).toEqual([]);
		expect(passes.after).toHaveLength(1);
		expect(passes.after[0]?.p[0]).toBeCloseTo(0.25);
		expect(passes.after[0]?.p[1]).toBe(2);
	});

	test("a blur too short to change anything is not drawn", () => {
		const passes = effectPasses(
			layer({ effects: [{ kind: "motion_blur", amount: 0.01 }] }),
			10,
			ctx,
		);
		expect(passes.before).toEqual([]);
	});

	test("the lengths follow the output scale", () => {
		const blur = layer({
			effects: [{ kind: "motion_blur", amount: 0.5, angle: 0 }],
		});
		const full = effectPasses(blur, 10, ctx);
		const half = effectPasses(blur, 10, { ...ctx, scale: 0.5 });
		expect(full.before[0]?.p[0]).toBe(15);
		expect(half.before[0]?.p[0]).toBe(7);
	});

	test("the frame past the end of the transition list has none", () => {
		const passes = effectPasses(
			layer({ transition: [{ kind: "dip", edge: "in", p: 100 }] }),
			20,
			ctx,
		);
		expect(passes.before).toEqual([]);
	});

	test("the grain counts frames from the start of the entry", () => {
		const passes = effectPasses(
			layer({ effects: [{ kind: "grain", amount: 0.3, size: 1, seed: 7 }] }),
			14,
			ctx,
		);
		expect(passes.before[0]?.p.slice(0, 4)).toEqual([19, 1, 7, 4]);
	});
});
