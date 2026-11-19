import { describe, expect, test } from "vitest";

import type { Layer } from "../api/types";
import { layerBoxes } from "./boxes";
import { moveBox, resizeBox, rotationTo } from "./gestures";
import { hitTest } from "./hit";
import { edgesOf, snap, snapTargets } from "./snapping";
import {
	clampZoom,
	effectiveZoom,
	fitZoom,
	origin,
	toDocument,
	zoomAbout,
} from "./viewport";

const layer = (extra: Partial<Layer> & { id: string }): Layer => ({
	kind: "shape",
	name: extra.id,
	visible: true,
	opacity: 100,
	x: 0,
	y: 0,
	w: 100,
	h: 50,
	...extra,
});

const VIEW = { zoom: 1, fit: true, panX: 0, panY: 0 };

describe("viewport", () => {
	test("fit leaves a margin and never zooms in past 100%", () => {
		expect(fitZoom({ w: 1000, h: 600 }, { w: 1280, h: 720 })).toBeCloseTo(
			Math.min(952 / 1280, 552 / 720),
		);
		expect(fitZoom({ w: 4000, h: 4000 }, { w: 100, h: 100 })).toBe(1);
	});

	test("zoom is clamped to 10% and 800%", () => {
		expect(clampZoom(0.01)).toBe(0.1);
		expect(clampZoom(50)).toBe(8);
	});

	test("zooming about a point keeps the document point under it", () => {
		const stage = { w: 1000, h: 600 };
		const doc = { w: 1280, h: 720 };
		const at = { x: 300, y: 200 };
		const before = toDocument(
			at,
			VIEW,
			stage,
			doc,
			effectiveZoom(VIEW, stage, doc),
		);
		const next = zoomAbout(VIEW, stage, doc, 2, at);
		expect(next.fit).toBe(false);
		const after = toDocument(at, next, stage, doc, next.zoom);
		expect(after.x).toBeCloseTo(before.x, 6);
		expect(after.y).toBeCloseTo(before.y, 6);
	});

	test("the picture is centred and the pan moves it", () => {
		const o = origin(
			{ ...VIEW, panX: 10, panY: -5 },
			{ w: 1000, h: 600 },
			{ w: 200, h: 100 },
			1,
		);
		expect(o).toEqual({ x: 410, y: 245 });
	});
});

describe("boxes and hit testing", () => {
	test("a child of a group is where the plan draws it, and the group is the union", () => {
		const boxes = layerBoxes([
			layer({
				id: "g",
				kind: "group",
				x: 10,
				y: 20,
				w: 0,
				h: 0,
				children: [
					layer({ id: "a", x: 5, y: 5 }),
					layer({ id: "b", x: 200, y: 0 }),
				],
			}),
		]);
		expect(boxes.find((b) => b.id === "a")).toMatchObject({
			x: 15,
			y: 25,
			offsetX: 10,
		});
		expect(boxes.find((b) => b.id === "g")).toMatchObject({
			x: 15,
			y: 20,
			w: 295,
			h: 55,
		});
	});

	test("hidden and locked state follow the groups", () => {
		const boxes = layerBoxes([
			layer({
				id: "g",
				kind: "group",
				visible: false,
				locked: true,
				children: [layer({ id: "a" })],
			}),
		]);
		expect(boxes.find((b) => b.id === "a")).toMatchObject({
			visible: false,
			locked: true,
		});
	});

	test("the topmost visible layer under the point wins; groups are never hit", () => {
		const boxes = layerBoxes([
			layer({ id: "under", w: 200, h: 200 }),
			layer({ id: "hidden", w: 200, h: 200, visible: false }),
			layer({ id: "over", x: 50, y: 50, w: 50, h: 50 }),
		]);
		expect(hitTest(boxes, { x: 60, y: 60 })?.id).toBe("over");
		expect(hitTest(boxes, { x: 10, y: 10 })?.id).toBe("under");
		expect(hitTest(boxes, { x: 500, y: 500 })).toBeUndefined();
	});

	test("a rotated layer is hit through the inverse rotation", () => {
		const boxes = layerBoxes([
			layer({ id: "bar", x: 0, y: 0, w: 100, h: 10, rotation: 90 }),
		]);
		// the box centre is (50, 5); turned by 90 degrees it is 10 wide and 100 tall about that centre
		expect(hitTest(boxes, { x: 50, y: 50 })?.id).toBe("bar");
		expect(hitTest(boxes, { x: 95, y: 5 })).toBeUndefined();
	});
});

describe("snapping", () => {
	const canvas = { w: 1280, h: 720 };

	test("edges and centre snap to the canvas within the tolerance in screen pixels", () => {
		const targets = snapTargets([], canvas, new Set());
		// at 100%: 4 px from the left edge snaps; at 25% the same box is 1 screen pixel away, so it snaps too
		expect(
			snap(edgesOf({ x: 4, y: 100, w: 100, h: 50 }), targets, 1),
		).toMatchObject({ dx: -4, guideX: 0 });
		expect(snap(edgesOf({ x: 30, y: 100, w: 100, h: 50 }), targets, 1).dx).toBe(
			0,
		);
		expect(
			snap(edgesOf({ x: 590, y: 100, w: 100, h: 50 }), targets, 1),
		).toMatchObject({ dx: 0, guideX: 640 });
	});

	test("the other layers are targets and the moving one is not", () => {
		const boxes = layerBoxes([
			layer({ id: "a", x: 300, y: 300 }),
			layer({ id: "b", x: 0, y: 0 }),
		]);
		const targets = snapTargets(boxes, canvas, new Set(["b"]));
		expect(targets.xs).toContain(300);
		expect(targets.xs).not.toContain(100);
	});

	test("the tolerance grows in canvas pixels as the view zooms out", () => {
		const targets = snapTargets([], canvas, new Set());
		expect(snap(edgesOf({ x: 10, y: 100, w: 100, h: 50 }), targets, 1).dx).toBe(
			0,
		);
		expect(
			snap(edgesOf({ x: 10, y: 100, w: 100, h: 50 }), targets, 0.5).dx,
		).toBe(-10);
	});
});

describe("gestures", () => {
	const box = { x: 100, y: 100, w: 200, h: 100, rotation: 0 };

	test("a move is whole pixels", () => {
		expect(moveBox(box, 10.4, -3.6)).toMatchObject({ x: 110, y: 96 });
	});

	test("the opposite corner stays where it is", () => {
		const grown = resizeBox(box, "se", { x: 50, y: 20 }, false);
		expect(grown).toMatchObject({ x: 100, y: 100, w: 250, h: 120 });
		const nw = resizeBox(box, "nw", { x: -20, y: -10 }, false);
		expect(nw).toMatchObject({ x: 80, y: 90, w: 220, h: 110 });
	});

	test("a corner keeps the aspect ratio on request", () => {
		const grown = resizeBox(box, "se", { x: 100, y: 0 }, true);
		expect(grown.w / grown.h).toBeCloseTo(2, 1);
	});

	test("a box cannot collapse", () => {
		expect(resizeBox(box, "e", { x: -1000, y: 0 }, false).w).toBe(1);
	});

	test("a rotated box grows along its own axes", () => {
		const turned = { ...box, rotation: 90 };
		// dragging the east handle straight down moves it along the box's east axis (which now points down)
		const grown = resizeBox(turned, "e", { x: 0, y: 40 }, false);
		expect(grown.w).toBe(240);
		expect(grown.h).toBe(100);
	});

	test("the rotation points the handle at the pointer, in steps on request", () => {
		expect(rotationTo(box, { x: 200, y: 0 }, 1)).toBe(0);
		expect(rotationTo(box, { x: 500, y: 150 }, 1)).toBe(90);
		expect(rotationTo(box, { x: 200, y: 400 }, 1)).toBe(180);
		expect(rotationTo(box, { x: 330, y: 20 }, 15) % 15).toBe(0);
	});
});
