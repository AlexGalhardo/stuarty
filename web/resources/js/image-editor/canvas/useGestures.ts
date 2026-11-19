import {
	type PointerEvent as ReactPointerEvent,
	useCallback,
	useRef,
	useState,
} from "react";

import { findLayer, useImageStore } from "../state/context";
import { type LayerBox, layerBoxes } from "./boxes";
import { type Moved, transformOps } from "./commands";
import {
	type Box,
	type Handle,
	moveBox,
	resizeBox,
	rotationTo,
} from "./gestures";
import { hitTest } from "./hit";
import { edgesOf, snap, snapTargets } from "./snapping";
import { effectiveZoom, type Point, toDocument } from "./viewport";

// What a drag is showing while the pointer is down: the outline of each layer at its transient box, and the
// guides of the snap. Nothing of it is sent until the pointer is released.
export interface Drag {
	boxes: ReadonlyMap<string, Box>;
	guideX: number | null;
	guideY: number | null;
}

type Mode =
	| { kind: "move" }
	| { kind: "resize"; handle: Handle }
	| { kind: "rotate" };

interface Active {
	mode: Mode;
	start: Point;
	ids: string[];
	startBoxes: Map<string, LayerBox>;
	targets: ReturnType<typeof snapTargets>;
	moved: boolean;
}

const asBox = (b: LayerBox): Box => ({
	x: b.x,
	y: b.y,
	w: b.w,
	h: b.h,
	rotation: b.rotation,
});

// The pointer gestures of the canvas: select, move, scale and rotate (SDD-012 section 24.4).
export function useGestures(stage: React.RefObject<HTMLDivElement | null>): {
	drag: Drag | null;
	onPointerDown(event: ReactPointerEvent<HTMLElement>): boolean;
	onPointerMove(event: ReactPointerEvent<HTMLElement>): void;
	onPointerUp(event: ReactPointerEvent<HTMLElement>): void;
	cancel(): void;
} {
	const store = useImageStore();
	const active = useRef<Active | null>(null);
	const [drag, setDrag] = useState<Drag | null>(null);
	const latest = useRef<Drag | null>(null);

	const docPoint = useCallback(
		(event: { clientX: number; clientY: number }): Point => {
			const el = stage.current;
			const { doc, view, stage: size } = store.getState();
			if (el === null || doc === null) return { x: 0, y: 0 };
			const rect = el.getBoundingClientRect();
			const dims = { w: doc.width, h: doc.height };
			const zoom = effectiveZoom(view, size, dims);
			return toDocument(
				{ x: event.clientX - rect.left, y: event.clientY - rect.top },
				view,
				size,
				dims,
				zoom,
			);
		},
		[store, stage],
	);

	const cancel = useCallback(() => {
		active.current = null;
		latest.current = null;
		setDrag(null);
	}, []);

	const onPointerDown = useCallback(
		(event: ReactPointerEvent<HTMLElement>): boolean => {
			if (event.button !== 0) return false;
			const state = store.getState();
			const doc = state.doc;
			if (doc === null || state.busy) return false;
			const boxes = layerBoxes(doc.layers);
			const byId = new Map(boxes.map((b) => [b.id, b]));
			const point = docPoint(event);
			const target = event.target as HTMLElement;
			const handle = target.dataset.handle;

			let mode: Mode | null = null;
			let ids = [...state.selection];

			if (handle !== undefined && ids.length === 1) {
				mode =
					handle === "rotate"
						? { kind: "rotate" }
						: { kind: "resize", handle: handle as Handle };
			} else {
				const hit = hitTest(boxes, point);
				if (hit === undefined) {
					if (!event.shiftKey) state.select([]);
					return true;
				}
				if (event.shiftKey) {
					ids = ids.includes(hit.id)
						? ids.filter((id) => id !== hit.id)
						: [...ids, hit.id];
					state.select(ids);
					return true;
				}
				if (!ids.includes(hit.id)) {
					ids = [hit.id];
					state.select(ids);
				}
				mode = { kind: "move" };
			}

			// A locked layer is selected but never dragged.
			const movable = ids.filter((id) => byId.get(id)?.locked === false);
			if (movable.length === 0) return true;

			active.current = {
				mode,
				start: point,
				ids: movable,
				startBoxes: new Map(
					movable.flatMap((id) => {
						const b = byId.get(id);
						return b === undefined ? [] : [[id, b] as const];
					}),
				),
				targets: snapTargets(
					boxes,
					{ w: doc.width, h: doc.height },
					new Set(movable),
				),
				moved: false,
			};
			(event.currentTarget as HTMLElement).setPointerCapture(event.pointerId);
			return true;
		},
		[store, docPoint],
	);

	const onPointerMove = useCallback(
		(event: ReactPointerEvent<HTMLElement>): void => {
			const a = active.current;
			if (a === null) return;
			const state = store.getState();
			const doc = state.doc;
			if (doc === null) return;
			const zoom = effectiveZoom(state.view, state.stage, {
				w: doc.width,
				h: doc.height,
			});
			const point = docPoint(event);
			let dx = point.x - a.start.x;
			let dy = point.y - a.start.y;
			if (Math.abs(dx) * zoom < 2 && Math.abs(dy) * zoom < 2 && !a.moved)
				return;
			a.moved = true;

			const out = new Map<string, Box>();
			let guideX: number | null = null;
			let guideY: number | null = null;
			const primary = a.startBoxes.get(a.ids[0] ?? "");
			if (primary === undefined) return;

			if (a.mode.kind === "move") {
				if (!event.altKey && a.ids.length > 0) {
					// The union of what moves snaps as one box.
					const union = unionOf(
						a.ids.map((id) => asBox(a.startBoxes.get(id) ?? primary)),
					);
					const moved = moveBox(union, dx, dy);
					const s = snap(edgesOf(moved), a.targets, zoom);
					dx += s.dx;
					dy += s.dy;
					guideX = s.guideX;
					guideY = s.guideY;
				}
				for (const id of a.ids) {
					const b = a.startBoxes.get(id);
					if (b !== undefined) out.set(id, moveBox(asBox(b), dx, dy));
				}
			} else if (a.mode.kind === "resize") {
				const start = asBox(primary);
				let delta = { x: dx, y: dy };
				const keep = event.shiftKey && a.mode.handle.length === 2;
				if (!event.altKey && start.rotation === 0) {
					const first = resizeBox(start, a.mode.handle, delta, keep);
					const edges = {
						xs: [] as number[],
						ys: [] as number[],
					};
					if (a.mode.handle.includes("e")) edges.xs.push(first.x + first.w);
					if (a.mode.handle.includes("w")) edges.xs.push(first.x);
					if (a.mode.handle.includes("s")) edges.ys.push(first.y + first.h);
					if (a.mode.handle.includes("n")) edges.ys.push(first.y);
					const s = snap(edges, a.targets, zoom);
					if (!keep) delta = { x: delta.x + s.dx, y: delta.y + s.dy };
					guideX = s.guideX;
					guideY = s.guideY;
				}
				out.set(primary.id, resizeBox(start, a.mode.handle, delta, keep));
			} else {
				const start = asBox(primary);
				out.set(primary.id, {
					...start,
					rotation: rotationTo(start, point, event.shiftKey ? 15 : 1),
				});
			}
			latest.current = { boxes: out, guideX, guideY };
			setDrag(latest.current);
		},
		[store, docPoint],
	);

	const onPointerUp = useCallback(
		(event: ReactPointerEvent<HTMLElement>): void => {
			const a = active.current;
			active.current = null;
			const current = latest.current;
			latest.current = null;
			setDrag(null);
			if (a === null || !a.moved || current === null) return;
			const doc = store.getState().doc;
			if (doc === null) return;
			const moved: Moved[] = [];
			for (const id of a.ids) {
				const before = a.startBoxes.get(id);
				const after = current.boxes.get(id);
				if (before === undefined || after === undefined) continue;
				const layer = findLayer(doc.layers, id);
				if (layer?.kind === "group") {
					moved.push({
						id,
						before: { x: layer.x, y: layer.y, w: 0, h: 0, rotation: 0 },
						after: {
							x: layer.x + (after.x - before.x),
							y: layer.y + (after.y - before.y),
							w: 0,
							h: 0,
							rotation: 0,
						},
						offsetX: 0,
						offsetY: 0,
						group: true,
					});
				} else {
					moved.push({
						id,
						before: asBox(before),
						after,
						offsetX: before.offsetX,
						offsetY: before.offsetY,
						group: false,
					});
				}
			}
			const ops = transformOps(moved);
			if (ops.length > 0) void store.getState().send(ops);
			(event.currentTarget as HTMLElement).releasePointerCapture?.(
				event.pointerId,
			);
		},
		[store],
	);

	return { drag, onPointerDown, onPointerMove, onPointerUp, cancel };
}

function unionOf(boxes: readonly Box[]): Box {
	const left = Math.min(...boxes.map((b) => b.x));
	const top = Math.min(...boxes.map((b) => b.y));
	const right = Math.max(...boxes.map((b) => b.x + b.w));
	const bottom = Math.max(...boxes.map((b) => b.y + b.h));
	return { x: left, y: top, w: right - left, h: bottom - top, rotation: 0 };
}
