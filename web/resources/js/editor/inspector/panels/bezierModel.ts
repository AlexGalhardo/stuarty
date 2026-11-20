// The curve editor of a keyframe segment (SDD-001 section 3.8, SDD-002 C.75): the
// four numbers x1, y1, x2, y2 of a cubic Bezier curve. This keeps a handle being
// dragged inside what the core accepts and maps the numbers to the drawing; the
// core judges the numbers and is the only place that evaluates the curve.

export type Curve = readonly [number, number, number, number];

// The limits of the four numbers, as in the specification: x1 and x2 from 0 to 1,
// y1 and y2 from -2 to 3.
export const X_RANGE = { min: 0, max: 1 } as const;
export const Y_RANGE = { min: -2, max: 3 } as const;

// What the plot shows of the y axis; a typed value beyond it is drawn on the edge.
export const VIEW_Y = { min: -1, max: 2 } as const;

export const SIZE = 200;
export const PAD = 24;
const SPAN = SIZE - 2 * PAD;

const clamp = (value: number, low: number, high: number): number =>
	Math.min(high, Math.max(low, value));

const round3 = (value: number): number => Math.round(value * 1000) / 1000;

export const clampX = (x: number): number =>
	round3(clamp(x, X_RANGE.min, X_RANGE.max));
export const clampY = (y: number): number =>
	round3(clamp(y, Y_RANGE.min, Y_RANGE.max));

// The stored four numbers as a curve, or null when the keyframe has none.
export function curveOf(numbers: readonly number[] | undefined): Curve | null {
	if (numbers === undefined || numbers.length !== 4) return null;
	const [x1, y1, x2, y2] = numbers as [number, number, number, number];
	return [x1, y1, x2, y2];
}

export const sameCurve = (a: Curve, b: Curve): boolean =>
	a.every((value, index) => value === b[index]);

// Moves a control point (0 is the first, 1 the second).
export function moveHandle(
	curve: Curve,
	handle: 0 | 1,
	x: number,
	y: number,
): Curve {
	const next: [number, number, number, number] = [...curve];
	next[handle * 2] = clampX(x);
	next[handle * 2 + 1] = clampY(y);
	return next;
}

// Sets one of the four numbers.
export function setNumber(curve: Curve, index: number, value: number): Curve {
	const next: [number, number, number, number] = [...curve];
	const isX = index % 2 === 0;
	next[index] = isX ? clampX(value) : clampY(value);
	return next;
}

export const toViewX = (x: number): number => PAD + x * SPAN;
export const toViewY = (y: number): number => {
	const shown = clamp(y, VIEW_Y.min, VIEW_Y.max);
	return SIZE - PAD - ((shown - VIEW_Y.min) / (VIEW_Y.max - VIEW_Y.min)) * SPAN;
};
export const fromViewX = (px: number): number => (px - PAD) / SPAN;
export const fromViewY = (py: number): number =>
	VIEW_Y.min + ((SIZE - PAD - py) / SPAN) * (VIEW_Y.max - VIEW_Y.min);

// The path a browser draws for the curve: from (0, 0) to (1, 1) through the two
// control points, as the SVG cubic command takes them. Drawing only.
export function curvePath(curve: Curve): string {
	const [x1, y1, x2, y2] = curve;
	return [
		`M${toViewX(0)} ${toViewY(0)}`,
		`C${toViewX(x1)} ${toViewY(y1)}`,
		`${toViewX(x2)} ${toViewY(y2)}`,
		`${toViewX(1)} ${toViewY(1)}`,
	].join(" ");
}
