import { describe, expect, test } from "vitest";

import {
	adjacentCut,
	adjacentMarker,
	clampPlayhead,
	clientXToFrame,
	DEFAULT_FPS,
	deltaPxToFrames,
	extentPx,
	fitZoom,
	formatRulerLabel,
	frameInItem,
	idsOnSide,
	nextZoom,
	rulerStep,
	selectionSpan,
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

describe("idsOnSide", () => {
	const items = [
		{ id: "a", start: 0, duration: 30 },
		{ id: "b", start: 30, duration: 20 },
		{ id: "c", start: 80, duration: 10 },
	];

	test("left holds what starts before the playhead, right what ends after it", () => {
		expect(idsOnSide(items, 40, "left")).toEqual(["a", "b"]);
		expect(idsOnSide(items, 40, "right")).toEqual(["b", "c"]);
		expect(idsOnSide(items, 30, "left")).toEqual(["a"]);
		expect(idsOnSide(items, 50, "right")).toEqual(["c"]);
		expect(idsOnSide(items, 0, "left")).toEqual([]);
	});
});

describe("adjacentMarker", () => {
	const frames = [40, 10, 90];

	test("finds the nearest marker strictly after or before", () => {
		expect(adjacentMarker(frames, 10, 1)).toBe(40);
		expect(adjacentMarker(frames, 0, 1)).toBe(10);
		expect(adjacentMarker(frames, 40, -1)).toBe(10);
		expect(adjacentMarker(frames, 95, -1)).toBe(90);
	});

	test("is null past the last or before the first marker", () => {
		expect(adjacentMarker(frames, 90, 1)).toBeNull();
		expect(adjacentMarker(frames, 10, -1)).toBeNull();
		expect(adjacentMarker([], 5, 1)).toBeNull();
	});
});

describe("adjacentCut", () => {
	const items = [
		{ start: 0, duration: 30 },
		{ start: 30, duration: 20 },
		{ start: 80, duration: 10 },
	];

	test("goes to the next cut after the playhead", () => {
		expect(adjacentCut(items, 0, 1)).toBe(30);
		expect(adjacentCut(items, 30, 1)).toBe(50);
		expect(adjacentCut(items, 50, 1)).toBe(80);
	});

	test("goes to the previous cut before the playhead", () => {
		expect(adjacentCut(items, 85, -1)).toBe(80);
		expect(adjacentCut(items, 80, -1)).toBe(50);
	});

	test("is null past the last or before the first cut", () => {
		expect(adjacentCut(items, 90, 1)).toBeNull();
		expect(adjacentCut(items, 0, -1)).toBeNull();
	});
});

describe("blade and range helpers", () => {
	test("frameInItem is the item start plus the pixels from its left edge", () => {
		expect(frameInItem(30, 160, 100, 2)).toBe(60);
		expect(frameInItem(30, 100, 100, 2)).toBe(30);
		expect(frameInItem(0, 0, 100, 2)).toBe(0);
	});

	test("selectionSpan covers the earliest start to the latest end", () => {
		expect(selectionSpan([])).toBeNull();
		expect(
			selectionSpan([
				{ id: "a", start: 30, duration: 20 },
				{ id: "b", start: 10, duration: 5 },
				{ id: "c", start: 40, duration: 30 },
			]),
		).toEqual({ in: 10, out: 70 });
	});
});
