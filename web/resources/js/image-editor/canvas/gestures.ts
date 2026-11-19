import type { Point } from "./viewport";

// The transient transform of a drag (SDD-012 section 24.4): where the outline of a layer is while the pointer
// is down. The result is only drawn; on release the integer box goes out as one `layer.transform`, and the
// answer of the server replaces what is on screen.

export interface Box {
	x: number;
	y: number;
	w: number;
	h: number;
	rotation: number;
}

export type Handle = "n" | "ne" | "e" | "se" | "s" | "sw" | "w" | "nw";

export const HANDLES: readonly Handle[] = [
	"nw",
	"n",
	"ne",
	"e",
	"se",
	"s",
	"sw",
	"w",
];

const RAD = Math.PI / 180;

function rotate(p: Point, degrees: number): Point {
	const c = Math.cos(degrees * RAD);
	const s = Math.sin(degrees * RAD);
	return { x: p.x * c - p.y * s, y: p.x * s + p.y * c };
}

export function moveBox(start: Box, dx: number, dy: number): Box {
	return { ...start, x: Math.round(start.x + dx), y: Math.round(start.y + dy) };
}

// The box after a drag of `handle` by `delta` (canvas pixels). The opposite edge or corner stays where it is,
// also for a rotated box; a corner keeps the aspect ratio when `keepAspect` is set.
export function resizeBox(
	start: Box,
	handle: Handle,
	delta: Point,
	keepAspect: boolean,
): Box {
	const local = rotate(delta, -start.rotation);
	let dw = 0;
	let dh = 0;
	if (handle.includes("e")) dw = local.x;
	if (handle.includes("w")) dw = -local.x;
	if (handle.includes("s")) dh = local.y;
	if (handle.includes("n")) dh = -local.y;

	let w = Math.max(1, start.w + dw);
	let h = Math.max(1, start.h + dh);
	if (keepAspect && handle.length === 2 && start.w > 0 && start.h > 0) {
		const ratio = Math.max(w / start.w, h / start.h);
		w = Math.max(1, start.w * ratio);
		h = Math.max(1, start.h * ratio);
	}

	// The centre moves by half of the growth, towards the side that moved, in the frame of the box.
	const grow = { x: 0, y: 0 };
	if (handle.includes("e")) grow.x = (w - start.w) / 2;
	if (handle.includes("w")) grow.x = -(w - start.w) / 2;
	if (handle.includes("s")) grow.y = (h - start.h) / 2;
	if (handle.includes("n")) grow.y = -(h - start.h) / 2;
	const shift = rotate(grow, start.rotation);
	const cx = start.x + start.w / 2 + shift.x;
	const cy = start.y + start.h / 2 + shift.y;

	const wr = Math.max(1, Math.round(w));
	const hr = Math.max(1, Math.round(h));
	return {
		x: Math.round(cx - wr / 2),
		y: Math.round(cy - hr / 2),
		w: wr,
		h: hr,
		rotation: start.rotation,
	};
}

// The angle, in whole degrees clockwise from straight up, that points the rotate handle at the pointer.
// `step` rounds it (15 with Shift).
export function rotationTo(start: Box, pointer: Point, step: number): number {
	const cx = start.x + start.w / 2;
	const cy = start.y + start.h / 2;
	let angle = Math.atan2(pointer.x - cx, -(pointer.y - cy)) / RAD;
	if (step > 1) angle = Math.round(angle / step) * step;
	angle = Math.round(angle);
	if (angle > 180) angle -= 360;
	if (angle < -180) angle += 360;
	return angle;
}
