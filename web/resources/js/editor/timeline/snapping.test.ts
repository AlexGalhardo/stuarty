import { describe, expect, test } from "vitest";

import type { SnapPoint } from "../api/types";
import { SNAP_PX, type SnapInput, snapFrame } from "./snapping";

const base: SnapInput = {
	frame: 500,
	ppf: 1,
	playhead: 1000,
	points: [],
	exclude: new Set(),
	enabled: true,
	suspended: false,
};

const edge = (frame: number, item?: string): SnapPoint => ({
	frame,
	kind: "item_end",
	item,
});

describe("snapFrame", () => {
	test("the threshold is 8 pixels at any zoom: 7 snaps, 9 does not", () => {
		for (const ppf of [0.25, 0.5, 1]) {
			const frames7 = 7 / ppf;
			const frames9 = 9 / ppf;
			const points = [edge(300)];
			const near = snapFrame({ ...base, ppf, frame: 300 + frames7, points });
			expect(near).toMatchObject({ frame: 300, line: 300, kind: "item_end" });
			const far = snapFrame({ ...base, ppf, frame: 300 + frames9, points });
			expect(far).toEqual({ frame: 300 + frames9, line: null, kind: null });
		}
		expect(SNAP_PX).toBe(8);
	});

	test("exactly 8 pixels snaps", () => {
		expect(snapFrame({ ...base, frame: 308, points: [edge(300)] }).frame).toBe(
			300,
		);
	});

	test("the nearest target wins", () => {
		const result = snapFrame({
			...base,
			frame: 304,
			points: [edge(300), edge(306)],
		});
		expect(result.frame).toBe(306);
	});

	test("precedence when equally close: playhead, then frame 0, then the core", () => {
		const points = [edge(4), edge(1000 + 4)];
		expect(snapFrame({ ...base, frame: 2, playhead: 4, points }).kind).toBe(
			"playhead",
		);
		expect(
			snapFrame({ ...base, frame: 2, playhead: 900, points: [edge(4)] }).kind,
		).toBe("timeline_start");
		expect(
			snapFrame({
				...base,
				frame: 502,
				playhead: 900,
				points: [edge(500), edge(504)],
			}),
		).toMatchObject({ frame: 500, kind: "item_end" });
		expect(
			snapFrame({ ...base, frame: 2, playhead: 2, points: [edge(2)] }).kind,
		).toBe("playhead");
	});

	test("points of the dragged items are excluded", () => {
		const points = [edge(300, "itm_a"), edge(310, "itm_b")];
		const result = snapFrame({
			...base,
			frame: 302,
			points,
			exclude: new Set(["itm_a"]),
		});
		expect(result.frame).toBe(310);
	});

	test("snapping off or suspended with Alt leaves the frame alone", () => {
		const points = [edge(500)];
		const frame = 503;
		for (const patch of [{ enabled: false }, { suspended: true }]) {
			expect(snapFrame({ ...base, frame, points, ...patch })).toEqual({
				frame,
				line: null,
				kind: null,
			});
		}
	});

	test("without core points the playhead and frame 0 still snap", () => {
		expect(snapFrame({ ...base, frame: 995, points: null }).frame).toBe(1000);
		expect(snapFrame({ ...base, frame: 5, points: null }).frame).toBe(0);
	});
});
