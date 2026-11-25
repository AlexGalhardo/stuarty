import { describe, expect, test } from "vitest";

import { advance } from "./scroll";

describe("the scroll of the teleprompter", () => {
	test("moves the text at the set speed in pixels a second", () => {
		expect(advance(0, 1000, 60, 5000)).toEqual({ offset: 60, done: false });
		expect(advance(0, 1000, 120, 5000).offset).toBe(120);
		expect(advance(100, 500, 80, 5000).offset).toBe(140);
	});

	test("a second at a speed is the same however many frames it is drawn in", () => {
		let offset = 0;
		for (let frame = 0; frame < 60; frame++) {
			offset = advance(offset, 1000 / 60, 90, 5000).offset;
		}
		expect(offset).toBeCloseTo(90, 6);
	});

	test("stops at the end of the script and says it is done", () => {
		expect(advance(4990, 1000, 60, 5000)).toEqual({ offset: 5000, done: true });
		expect(advance(0, 1000, 60, 0)).toEqual({ offset: 0, done: true });
	});

	test("never moves back, whatever the clock says", () => {
		expect(advance(10, -500, 60, 5000).offset).toBe(10);
	});
});
