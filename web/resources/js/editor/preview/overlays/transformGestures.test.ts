import { describe, expect, test } from "vitest";

import type { LayerGeometry } from "../types";
import {
	distance,
	nextRotation,
	nextScale,
	pointerAngle,
	previewGeometry,
	ROTATE_HANDLE_REACH,
	rotateHandleInside,
	turnBetween,
} from "./transformGestures";

const box: LayerGeometry = {
	item: "itm_a",
	cx: 100,
	cy: 100,
	w: 40,
	h: 20,
	rotation: 10,
	scale: 1,
	opacity: 1,
};

describe("transform gestures (screen arithmetic)", () => {
	test("the body moves the centre and nothing else", () => {
		expect(
			previewGeometry(box, "body", { dx: 5, dy: -5, factor: 3, turn: 90 }),
		).toEqual({ ...box, cx: 105, cy: 95 });
	});

	test("a corner scales the size about the centre", () => {
		expect(
			previewGeometry(box, "scale", { dx: 9, dy: 9, factor: 2, turn: 0 }),
		).toEqual({ ...box, w: 80, h: 40 });
	});

	test("the rotation handle turns the box", () => {
		expect(
			previewGeometry(box, "rotate", { dx: 0, dy: 0, factor: 1, turn: 35 }),
		).toEqual({ ...box, rotation: 45 });
	});

	test("the pointer angle is clockwise with y down", () => {
		const centre = { x: 0, y: 0 };
		expect(pointerAngle(centre, { x: 1, y: 0 })).toBe(0);
		expect(pointerAngle(centre, { x: 0, y: 1 })).toBe(90);
		expect(distance(centre, { x: 3, y: 4 })).toBe(5);
	});

	test("a turn takes the short way round", () => {
		expect(turnBetween(170, -170)).toBe(20);
		expect(turnBetween(-170, 170)).toBe(-20);
	});

	test("the scale stays inside what the core accepts, to 3 decimals", () => {
		expect(nextScale(1, 1.23456)).toBe(1.235);
		expect(nextScale(1, 100)).toBe(20);
		expect(nextScale(1, 0)).toBe(0.01);
	});

	test("the rotation is normalised to a half turn each way", () => {
		expect(nextRotation(170, 30)).toBe(-160);
		expect(nextRotation(0, 45.12345)).toBe(45.123);
	});
});

describe("rotation handle placement", () => {
	test("moves inside the box when its top is within the reach of the canvas edge", () => {
		expect(rotateHandleInside(0)).toBe(true);
		expect(rotateHandleInside(ROTATE_HANDLE_REACH - 1)).toBe(true);
		expect(rotateHandleInside(ROTATE_HANDLE_REACH)).toBe(false);
		expect(rotateHandleInside(200)).toBe(false);
	});
});
