import type { HandleKind, LayerGeometry } from "../types";

// The canvas gestures of SDD-012 sections 8.2 and 11.6: the corner handle, the
// rotation handle and the body. While the pointer moves only the outline box
// follows it (`onPreview`); the picture and the compositor are not touched.
// The pointer handling and the send are in timeline/gestures/transformDrag.ts
// (the import rules of SDD-012 section 3.3 keep `preview` free of `commands`).

export const MIN_SCALE = 0.01;
const MAX_SCALE = 20;

export const round3 = (value: number): number =>
	Math.round(value * 1000) / 1000;

// Degrees, clockwise, of the pointer around the centre, screen y pointing down.
export function pointerAngle(
	centre: { x: number; y: number },
	pointer: { x: number; y: number },
): number {
	return (
		(Math.atan2(pointer.y - centre.y, pointer.x - centre.x) * 180) / Math.PI
	);
}

export function distance(
	a: { x: number; y: number },
	b: { x: number; y: number },
): number {
	return Math.hypot(a.x - b.x, a.y - b.y);
}

// The turn from the pointer-down angle to the current one, kept in (-180, 180].
export function turnBetween(from: number, to: number): number {
	let turn = (to - from) % 360;
	if (turn > 180) turn -= 360;
	if (turn <= -180) turn += 360;
	return turn;
}

// The box as the gesture shows it, from the box at pointer down. `factor` is
// the ratio of the pointer distances from the centre.
export function previewGeometry(
	start: LayerGeometry,
	kind: HandleKind,
	input: { dx: number; dy: number; factor: number; turn: number },
): LayerGeometry {
	if (kind === "body") {
		return { ...start, cx: start.cx + input.dx, cy: start.cy + input.dy };
	}
	if (kind === "scale") {
		return {
			...start,
			w: start.w * input.factor,
			h: start.h * input.factor,
		};
	}
	return { ...start, rotation: start.rotation + input.turn };
}

// The value of the new scale: the scale at pointer down times the growth of the
// box, inside what the core accepts.
export function nextScale(base: number, factor: number): number {
	return round3(Math.min(MAX_SCALE, Math.max(MIN_SCALE, base * factor)));
}

export function nextRotation(base: number, turn: number): number {
	return round3(turnBetween(0, base + turn));
}

// The rotation handle hangs this many screen pixels above the box. A box whose
// top edge is closer than that to the top of the canvas would put the handle
// under the toolbar above the preview, where no pointer reaches it.
export const ROTATE_HANDLE_REACH = 36;

export function rotateHandleInside(boxTop: number): boolean {
	return boxTop < ROTATE_HANDLE_REACH;
}
