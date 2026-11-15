import { describe, expect, test } from "vitest";

import { frameWindow, mountedIds, VIRTUALIZE_ABOVE } from "./virtualize";

// 40 one-second clips of 30 frames, back to back.
const track = Array.from({ length: 40 }, (_, i) => ({
	id: `itm_${String(i).padStart(2, "0")}`,
	start: i * 30,
	duration: 30,
}));
const none = new Set<string>();

describe("frameWindow", () => {
	test("spans one viewport before and two after the scroll position", () => {
		// 1000 px viewport, 2 px per frame, scrolled to 3000 px.
		expect(frameWindow(3000, 1000, 2)).toEqual({ from: 1000, to: 2500 });
	});

	test("never starts before the first frame", () => {
		expect(frameWindow(200, 1000, 1).from).toBe(0);
	});

	test("small scrolls give the same window", () => {
		expect(frameWindow(2990, 1000, 2)).toEqual(frameWindow(2900, 1000, 2));
	});

	test("an unmeasured viewport still yields a window", () => {
		const w = frameWindow(0, 0, 1);
		expect(w.from).toBe(0);
		expect(w.to).toBeGreaterThan(0);
	});

	test("the threshold is the documented one", () => {
		expect(VIRTUALIZE_ABOVE).toBe(150);
	});
});

describe("mountedIds", () => {
	test("a null window mounts every item", () => {
		expect(mountedIds(track, null, none)).toHaveLength(40);
	});

	test("mounts the items that intersect the window and no others", () => {
		const ids = mountedIds(track, { from: 300, to: 600 }, none);
		expect(ids[0]).toBe("itm_10");
		expect(ids.at(-1)).toBe("itm_19");
		expect(ids).toHaveLength(10);
	});

	test("an item that ends exactly at the window start is out", () => {
		const ids = mountedIds(track, { from: 30, to: 60 }, none);
		expect(ids).toEqual(["itm_01"]);
	});

	test("an item that starts exactly at the window end is out", () => {
		const ids = mountedIds(track, { from: 0, to: 30 }, none);
		expect(ids).toEqual(["itm_00"]);
	});

	test("an item wider than the window is mounted", () => {
		const wide = [{ id: "a", start: 0, duration: 10_000 }];
		expect(mountedIds(wide, { from: 500, to: 900 }, none)).toEqual(["a"]);
	});

	test("pinned items stay mounted outside the window, in display order", () => {
		const ids = mountedIds(
			track,
			{ from: 300, to: 360 },
			new Set(["itm_39", "itm_00"]),
		);
		expect(ids).toEqual(["itm_00", "itm_10", "itm_11", "itm_39"]);
	});

	test("an empty track mounts nothing", () => {
		expect(mountedIds([], { from: 0, to: 100 }, none)).toEqual([]);
	});
});
