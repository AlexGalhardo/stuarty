import { describe, expect, test } from "vitest";

import executorSource from "./PixiExecutor.ts?raw";
import { stillFrameRect } from "./stillFrame";

describe("preview still crop", () => {
	test("a partial crop is not the whole thumbnail", () => {
		const bitmap = { width: 120, height: 120 };
		const whole = {
			x: 0,
			y: 0,
			width: bitmap.width,
			height: bitmap.height,
		};
		const got = stillFrameRect([0, 0, 60, 120], bitmap.width, bitmap.height);
		expect(got).toEqual({ x: 0, y: 0, width: 60, height: 120 });
		expect(got).not.toEqual(whole);
	});

	test("a poster of a wider source scales the crop onto the bitmap", () => {
		const got = stillFrameRect([0, 0, 2000, 3000], 640, 480, 4000, 3000);
		expect(got).toEqual({ x: 0, y: 0, width: 320, height: 480 });
		expect(got.width).toBeLessThan(640);
	});

	test("an uncropped still keeps the whole poster", () => {
		expect(stillFrameRect([0, 0, 120, 120], 120, 120)).toEqual({
			x: 0,
			y: 0,
			width: 120,
			height: 120,
		});
	});

	test("the still path returns that frame, not the full thumbnail", () => {
		const start = executorSource.indexOf("private still(");
		const end = executorSource.indexOf("private loadBitmap(");
		expect(start).toBeGreaterThanOrEqual(0);
		expect(end).toBeGreaterThan(start);
		const body = executorSource.slice(start, end);
		expect(body).toContain("cropAt(layer, frame)");
		expect(body).toContain("stillFrameRect(");
		expect(body).toContain("new Rectangle(");
		expect(body).toContain("return texture");
		expect(body).not.toContain(
			"new Texture({ source: new ImageSource({ resource: image }) })",
		);
	});
});
