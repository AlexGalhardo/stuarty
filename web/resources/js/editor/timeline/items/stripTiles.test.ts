import { describe, expect, test } from "vitest";

import {
	type StripItem,
	type StripLayout,
	stripRows,
	stripTiles,
	tileIndexAt,
} from "./stripTiles";

const fps = { num: 30, den: 1 };
const strip: StripLayout = {
	tile_w: 72,
	tile_h: 128,
	columns: 20,
	count: 60,
	interval_ms: 1000,
};

const combos: { name: string; item: StripItem }[] = [];
for (const speed of [
	{ num: 1, den: 4 },
	{ num: 1, den: 2 },
	{ num: 1, den: 1 },
	{ num: 3, den: 2 },
	{ num: 4, den: 1 },
]) {
	for (const [source_in, duration] of [
		[0, 150],
		[90, 300],
		[1700, 600],
		[1790, 5000],
	] as const) {
		combos.push({
			name: `speed ${speed.num}/${speed.den}, in ${source_in}, ${duration} frames`,
			item: { source_in, duration, speed },
		});
	}
}

describe("stripTiles", () => {
	test("has the 20 combinations of the assert", () => {
		expect(combos).toHaveLength(20);
	});

	test.each(combos)(
		"the tile index stays inside the sheet: $name",
		({ item }) => {
			for (const frame of [
				-50,
				0,
				1,
				item.duration / 2,
				item.duration - 1,
				item.duration * 40,
			]) {
				const index = tileIndexAt(strip, fps, item, frame);
				expect(index).toBeGreaterThanOrEqual(0);
				expect(index).toBeLessThanOrEqual(strip.count - 1);
			}
			for (const tile of stripTiles(strip, fps, item)) {
				expect(tile.index).toBeGreaterThanOrEqual(0);
				expect(tile.index).toBeLessThanOrEqual(strip.count - 1);
				expect(tile.column).toBeLessThan(strip.columns);
				expect(tile.row).toBeLessThan(stripRows(strip));
			}
		},
	);

	test("a 5 s clip at normal speed shows five tiles of one second", () => {
		const tiles = stripTiles(strip, fps, { source_in: 0, duration: 150 });
		expect(tiles.map((tile) => tile.index)).toEqual([0, 1, 2, 3, 4]);
		expect(tiles.every((tile) => tile.length === 30)).toBe(true);
		expect(tiles[2]?.offset).toBe(60);
	});

	test("the first tile starts before an in point inside an interval", () => {
		const tiles = stripTiles(strip, fps, { source_in: 45, duration: 60 });
		expect(tiles[0]).toMatchObject({ index: 1, offset: -15 });
	});

	test("at 4x a tile lasts a quarter as long, at 0.25x four times as long", () => {
		expect(
			stripTiles(strip, fps, { duration: 30, speed: { num: 4, den: 1 } })[0]
				?.length,
		).toBe(7.5);
		expect(
			stripTiles(strip, fps, { duration: 30, speed: { num: 1, den: 4 } })[0]
				?.length,
		).toBe(120);
	});

	test("rows and columns place a tile row by row", () => {
		const tiles = stripTiles(strip, fps, { source_in: 1500, duration: 30 });
		expect(tiles[0]).toMatchObject({ index: 50, column: 10, row: 2 });
		expect(stripRows(strip)).toBe(3);
	});

	test("an empty sheet draws nothing", () => {
		expect(stripTiles({ ...strip, count: 0 }, fps, { duration: 30 })).toEqual(
			[],
		);
	});
});
