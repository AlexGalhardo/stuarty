import type { SnapPoint } from "../api/types";

// Snapping of SDD-012 section 7.7: screen arithmetic on points the core
// supplied. The nearest target within SNAP_PX pixels wins and the UI sends its
// frame; it decides nothing about what the edit does. This file is on the
// allowlist of rule R5.

export const SNAP_PX = 8;

export type SnapKind = "playhead" | SnapPoint["kind"];

export interface SnapInput {
	// The frame the pointer asks for.
	frame: number;
	ppf: number;
	playhead: number;
	// The points of the core; null while they are not usable (not fetched yet, or
	// of another revision), in which case only the playhead and frame 0 apply.
	points: readonly SnapPoint[] | null;
	// Items being dragged: their own edges are not targets.
	exclude: ReadonlySet<string>;
	// The Snap toggle.
	enabled: boolean;
	// `Alt` held: snapping is suspended for this move.
	suspended: boolean;
}

export interface SnapResult {
	frame: number;
	// The frame to draw the snap line at, or null when nothing snapped.
	line: number | null;
	kind: SnapKind | null;
}

// Candidates in order of precedence when equally close: the playhead, frame 0,
// then the core points in the order the core listed them.
function* targets(
	input: SnapInput,
): Generator<{ frame: number; kind: SnapKind }> {
	yield { frame: input.playhead, kind: "playhead" };
	yield { frame: 0, kind: "timeline_start" };
	for (const point of input.points ?? []) {
		if (point.item !== undefined && input.exclude.has(point.item)) continue;
		yield { frame: point.frame, kind: point.kind };
	}
}

export function snapFrame(input: SnapInput): SnapResult {
	const free: SnapResult = { frame: input.frame, line: null, kind: null };
	if (!input.enabled || input.suspended) return free;
	let best: { frame: number; kind: SnapKind; distance: number } | null = null;
	for (const target of targets(input)) {
		const distance = Math.abs(target.frame - input.frame) * input.ppf;
		if (distance > SNAP_PX) continue;
		if (best === null || distance < best.distance) {
			best = { ...target, distance };
		}
	}
	if (best === null) return free;
	return { frame: best.frame, line: best.frame, kind: best.kind };
}
