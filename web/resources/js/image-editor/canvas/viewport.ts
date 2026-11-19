import type { Viewport } from "../state/store";

// Screen arithmetic of the canvas (SDD-012 section 1.1, kind one): where the picture is drawn in the stage and
// how a point of the stage maps to a point of the document. None of it is sent to the server.

export const MIN_ZOOM = 0.1;
export const MAX_ZOOM = 8;
export const ZOOM_STEP = 1.25;
const FIT_MARGIN = 24;

export interface Size {
	w: number;
	h: number;
}

export interface Point {
	x: number;
	y: number;
}

export function clampZoom(zoom: number): number {
	return Math.min(MAX_ZOOM, Math.max(MIN_ZOOM, zoom));
}

// The zoom that shows the whole document in the stage, with a margin. Never above 100%.
export function fitZoom(stage: Size, doc: Size): number {
	if (doc.w <= 0 || doc.h <= 0) return 1;
	const w = Math.max(stage.w - 2 * FIT_MARGIN, 1);
	const h = Math.max(stage.h - 2 * FIT_MARGIN, 1);
	return clampZoom(Math.min(w / doc.w, h / doc.h, 1));
}

// The zoom in force: the fit zoom while `fit` is on, the chosen one otherwise.
export function effectiveZoom(view: Viewport, stage: Size, doc: Size): number {
	return view.fit ? fitZoom(stage, doc) : view.zoom;
}

// The top-left corner of the picture in the stage: centred, then moved by the pan.
export function origin(
	view: Viewport,
	stage: Size,
	doc: Size,
	zoom: number,
): Point {
	return {
		x: (stage.w - doc.w * zoom) / 2 + view.panX,
		y: (stage.h - doc.h * zoom) / 2 + view.panY,
	};
}

export function toDocument(
	point: Point,
	view: Viewport,
	stage: Size,
	doc: Size,
	zoom: number,
): Point {
	const o = origin(view, stage, doc, zoom);
	return { x: (point.x - o.x) / zoom, y: (point.y - o.y) / zoom };
}

// The viewport after a zoom by `factor` that keeps the point of the document under `at` (a point of the stage)
// where it is.
export function zoomAbout(
	view: Viewport,
	stage: Size,
	doc: Size,
	factor: number,
	at: Point,
): Viewport {
	const before = effectiveZoom(view, stage, doc);
	const next = clampZoom(before * factor);
	const anchor = toDocument(at, view, stage, doc, before);
	const centred = origin({ ...view, panX: 0, panY: 0 }, stage, doc, next);
	return {
		zoom: next,
		fit: false,
		panX: at.x - anchor.x * next - centred.x,
		panY: at.y - anchor.y * next - centred.y,
	};
}
