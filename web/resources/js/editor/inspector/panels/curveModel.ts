import type { CurvePointValue } from "../../commands/builders";

// The bounds of SDD-001 section 3.5: a curve has 2 to 8 points, x strictly
// increasing from 0 to 1. This only keeps a point being dragged inside what the
// core accepts; the core still judges the list and draws nothing here.
export const MIN_CURVE_POINTS = 2;
export const MAX_CURVE_POINTS = 8;
export const MIN_GAP = 0.001;

export const NEUTRAL_CURVE: readonly CurvePointValue[] = [
	{ x: 0, y: 0 },
	{ x: 1, y: 1 },
];

const clamp = (value: number, low: number, high: number): number =>
	Math.min(high, Math.max(low, value));

const round3 = (value: number): number => Math.round(value * 1000) / 1000;

// Moves one point; its x stays between the x of its neighbours.
export function movePoint(
	points: readonly CurvePointValue[],
	index: number,
	x: number,
	y: number,
): CurvePointValue[] {
	const before = points[index - 1];
	const after = points[index + 1];
	const low = before === undefined ? 0 : before.x + MIN_GAP;
	const high = after === undefined ? 1 : after.x - MIN_GAP;
	return points.map((point, at) =>
		at === index
			? { x: round3(clamp(x, low, high)), y: round3(clamp(y, 0, 1)) }
			: point,
	);
}

// Adds a point; refused (the same list) at 8 points or on top of another x.
export function addPoint(
	points: readonly CurvePointValue[],
	x: number,
	y: number,
): CurvePointValue[] {
	const px = round3(clamp(x, 0, 1));
	if (
		points.length >= MAX_CURVE_POINTS ||
		points.some((point) => Math.abs(point.x - px) < MIN_GAP)
	) {
		return [...points];
	}
	const next = { x: px, y: round3(clamp(y, 0, 1)) };
	const at = points.findIndex((point) => point.x > px);
	return at < 0
		? [...points, next]
		: [...points.slice(0, at), next, ...points.slice(at)];
}

// The "Add point" button: the middle of the widest gap, on the straight line
// between its two neighbours.
export function addPointInWidestGap(
	points: readonly CurvePointValue[],
): CurvePointValue[] {
	let widest = -1;
	let gap = 0;
	for (let at = 0; at + 1 < points.length; at++) {
		const here = (points[at + 1]?.x ?? 0) - (points[at]?.x ?? 0);
		if (here > gap) {
			gap = here;
			widest = at;
		}
	}
	const left = points[widest];
	const right = points[widest + 1];
	if (left === undefined || right === undefined) return [...points];
	return addPoint(points, (left.x + right.x) / 2, (left.y + right.y) / 2);
}

// Removes a point; refused (the same list) at 2 points.
export function removePoint(
	points: readonly CurvePointValue[],
	index: number,
): CurvePointValue[] {
	if (points.length <= MIN_CURVE_POINTS) return [...points];
	return points.filter((_, at) => at !== index);
}

export const sameCurve = (
	a: readonly CurvePointValue[],
	b: readonly CurvePointValue[],
): boolean =>
	a.length === b.length &&
	a.every((point, at) => point.x === b[at]?.x && point.y === b[at]?.y);
