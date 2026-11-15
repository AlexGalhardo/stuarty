// The thumbnail strip of a clip (SDD-012 sections 7.4 and 1.3): which tile of
// the sprite sheet of the asset sits where along the item. A decoration: it
// reads `source_in`, `speed`, `start` and `duration`, is never shown as a time
// and is never sent to the core, so a tile off by a frame costs a slightly
// misplaced picture and nothing else.
import type { MediaEntry } from "../../api/types";

export type StripLayout = NonNullable<MediaEntry["strip"]>;

export interface StripItem {
	source_in?: number;
	speed?: { num: number; den: number };
	duration: number;
}

export interface StripRate {
	num: number;
	den: number;
}

export interface StripTile {
	// The tile of the sheet, always in [0, count - 1].
	index: number;
	column: number;
	row: number;
	// Where the tile sits on the item, in timeline frames from its left edge
	// (negative when the tile starts before the in point), and how long it lasts.
	offset: number;
	length: number;
}

// The sheet holds a tile per `interval_ms` of the asset; project frames of one
// interval at the project rate.
function intervalFrames(strip: StripLayout, fps: StripRate): number {
	return (strip.interval_ms * fps.num) / (1000 * fps.den);
}

function speedOf(item: StripItem): number {
	return item.speed === undefined ? 1 : item.speed.num / item.speed.den;
}

// The tile shown at `frame` frames from the left edge of the item: tile
// `min(count - 1, floor(t / I))` of SDD-011 section 4.5, `t` being the asset
// position at that frame.
export function tileIndexAt(
	strip: StripLayout,
	fps: StripRate,
	item: StripItem,
	frame: number,
): number {
	const source = (item.source_in ?? 0) + frame * speedOf(item);
	const raw = Math.floor(source / intervalFrames(strip, fps));
	return Math.min(strip.count - 1, Math.max(0, raw));
}

// One tile element per interval of the asset that the item shows. The count is
// bounded by the sheet (at most 400), never by the zoom, so a zoom change moves
// the tiles through CSS and re-renders nothing.
export function stripTiles(
	strip: StripLayout,
	fps: StripRate,
	item: StripItem,
): StripTile[] {
	if (strip.count < 1 || strip.columns < 1 || item.duration <= 0) return [];
	const per = intervalFrames(strip, fps);
	const speed = speedOf(item);
	const from = item.source_in ?? 0;
	const to = from + item.duration * speed;
	const first = Math.min(strip.count - 1, Math.max(0, Math.floor(from / per)));
	const last = Math.min(
		strip.count - 1,
		Math.max(first, Math.ceil(to / per) - 1),
	);
	const tiles: StripTile[] = [];
	for (let index = first; index <= last; index++) {
		tiles.push({
			index,
			column: index % strip.columns,
			row: Math.floor(index / strip.columns),
			offset: (index * per - from) / speed,
			length: per / speed,
		});
	}
	return tiles;
}

export function stripRows(strip: StripLayout): number {
	return Math.ceil(strip.count / strip.columns);
}
