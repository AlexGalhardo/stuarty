import { describe, expect, test } from "vitest";

import {
	clampPlayhead,
	clientXToFrame,
	DEFAULT_FPS,
	deltaPxToFrames,
	extentPx,
	fitZoom,
	formatRulerLabel,
	nextZoom,
	rulerStep,
	sortByStart,
	TIMELINE_MAX_FRAME,
	ZOOM_LEVELS,
	zoomAround,
} from "./geometry";

describe("coordinates", () => {
	test("clientXToFrame maps pointer to frame and clamps", () => {
		expect(clientXToFrame(300, 100, 50, 2)).toBe(125);
		expect(clientXToFrame(0, 100, 0, 2)).toBe(0);
		expect(clientXToFrame(1e12, 0, 0, 1)).toBe(TIMELINE_MAX_FRAME);
	});

	test("deltaPxToFrames rounds", () => {
		expect(deltaPxToFrames(25, 2)).toBe(13);
		expect(deltaPxToFrames(-25, 2)).toBe(-12);
	});

	test("zoomAround keeps the frame under the pointer", () => {
		const { scrollLeft } = zoomAround(100, 200, 4);
		expect(scrollLeft).toBe(200);
		expect(scrollLeft + 200).toBe(100 * 4);
		expect(zoomAround(1, 500, 1).scrollLeft).toBe(0);
	});

	test("clampPlayhead rounds and clamps", () => {
		expect(clampPlayhead(-3)).toBe(0);
		expect(clampPlayhead(12.4)).toBe(12);
		expect(clampPlayhead(1e9)).toBe(TIMELINE_MAX_FRAME);
	});
});

describe("zoom", () => {
	test("nextZoom walks the levels and stops at the ends", () => {
		expect(nextZoom(1, 1)).toBe(2);
		expect(nextZoom(1, -1)).toBe(0.5);
		expect(nextZoom(0.7, 1)).toBe(1);
		expect(nextZoom(0.7, -1)).toBe(0.5);
		expect(nextZoom(32, 1)).toBe(32);
		expect(nextZoom(0.03125, -1)).toBe(0.03125);
		expect(ZOOM_LEVELS.length).toBe(11);
	});

	test("fitZoom fits the end with 5 percent slack, within the levels", () => {
		expect(fitZoom(1050, 1000)).toBe(1);
		expect(fitZoom(1000, 0)).toBe(1);
		expect(fitZoom(10, 100000)).toBe(0.03125);
		expect(fitZoom(1e6, 10)).toBe(32);
	});

	test("extentPx keeps empty space past the end", () => {
		expect(extentPx(800, 1000, 1)).toBe(1300);
		expect(extentPx(800, 0, 0.5)).toBe(800);
	});
});

describe("ruler", () => {
	test("labels are at least 70 px apart and use m:ss from one second", () => {
		const step = rulerStep(1, DEFAULT_FPS);
		expect(step.labelEvery * 1).toBeGreaterThanOrEqual(70);
		expect(step.labelEvery).toBe(150);
		expect(step.format).toBe("m:ss");
	});

	test("zoomed in, labels step in frames and read s+ff", () => {
		const step = rulerStep(32, DEFAULT_FPS);
		expect(step.labelEvery).toBe(5);
		expect(step.format).toBe("s+ff");
		expect(step.tickEvery).toBe(1);
	});

	test("labels", () => {
		expect(formatRulerLabel(150, DEFAULT_FPS, "m:ss")).toBe("0:05");
		expect(formatRulerLabel(3900, DEFAULT_FPS, "m:ss")).toBe("2:10");
		expect(formatRulerLabel(375, DEFAULT_FPS, "s+ff")).toBe("12+15");
	});

	test("NTSC rate steps are whole frames", () => {
		const step = rulerStep(0.5, { num: 30000, den: 1001 });
		expect(Number.isInteger(step.labelEvery)).toBe(true);
	});
});

describe("display order", () => {
	test("sortByStart orders by start then id", () => {
		const sorted = sortByStart([
			{ id: "b", start: 10, duration: 1 },
			{ id: "a", start: 10, duration: 1 },
			{ id: "c", start: 0, duration: 1 },
		]);
		expect(sorted.map((i) => i.id)).toEqual(["c", "a", "b"]);
	});
});
