import type {
	PlanBitmapLayer,
	PlanLayer,
	PlanMediaLayer,
	PlanWindow,
} from "../../api/types";
import {
	bitmapKeyAt,
	colorMatrixAt,
	cropAt,
	maskAt,
	valueAt,
} from "../planValue";
import type { RenderInput } from "./protocol";

// The expansion of SDD-013 section 2.4: a plan window and a frame give the
// layers of the render input of that frame. It copies and indexes what the core
// planned (`valueAt` takes the element `frame - first` of a per-frame series);
// it works nothing out, so the renderer draws what the export draws.

export interface OutputSize {
	width: number;
	height: number;
	// The output scale of SDD-004 section 6: the buffer width over the project's.
	scale: number;
}

type Json = Record<string, unknown>;

// Fields every layer kind shares, with the names of `core/plan` (SDD-013 2.2).
function common(layer: PlanLayer, frame: number): Json {
	const first = layer.first;
	const out: Json = {
		item: layer.item,
		cx: valueAt(layer.cx, frame, first),
		cy: valueAt(layer.cy, frame, first),
		w: valueAt(layer.w, frame, first),
		h: valueAt(layer.h, frame, first),
		rotation: valueAt(layer.rotation, frame, first),
		scale: valueAt(layer.scale, frame, first),
		opacity: valueAt(layer.opacity, frame, first),
	};
	if (layer.blend !== undefined && layer.blend !== "normal") {
		out.blend = layer.blend;
	}
	const mask = maskAt(layer, frame);
	if (mask !== null) out.mask = mask;
	if (layer.flip_x === true) out.flip_x = true;
	if (layer.flip_y === true) out.flip_y = true;
	if (layer.effects !== undefined && layer.effects.length > 0) {
		out.effects = layer.effects;
	}
	const transition = layer.transition?.[frame - first];
	if (transition !== undefined && transition !== null) {
		out.transition = transition;
	}
	if (layer.shadow !== undefined) out.shadow = layer.shadow;
	if (layer.outline !== undefined) out.outline = layer.outline;
	return out;
}

function mediaLayer(layer: PlanMediaLayer, frame: number): Json {
	const out = common(layer, frame);
	out.kind = "media";
	out.asset = layer.asset;
	out.asset_kind = layer.asset_kind;
	const crop = cropAt(layer, frame);
	if (crop !== null) {
		out.crop = { x: crop[0], y: crop[1], w: crop[2], h: crop[3] };
	}
	const matrix = colorMatrixAt(layer, frame);
	if (matrix !== null) out.color_matrix = matrix;
	if (layer.filter !== undefined && layer.filter !== null) {
		out.filter = layer.filter;
	}
	if (layer.blur !== undefined && layer.blur > 0) out.blur = layer.blur;
	if (layer.grade !== undefined) out.grade = layer.grade;
	if (layer.lut !== undefined) out.lut = layer.lut;
	if (layer.chroma_key !== undefined) out.chroma_key = layer.chroma_key;
	return out;
}

// The bitmap layer's `bitmap` object is only checked for its kind until the
// rasterizer moves into the renderer (SDD-013 section 2.5): the pixels are the
// ones uploaded under `bitmap_key`.
function bitmapLayer(layer: PlanBitmapLayer, frame: number, key: string): Json {
	const out = common(layer, frame);
	out.kind = "bitmap";
	out.bitmap = { kind: "text" };
	out.bitmap_key = key;
	return out;
}

// The render input of one frame for the layers whose pixels are ready, in plan
// order. `layers` is a subset of the layers of the window at `frame`.
export function frameInput(
	plan: Pick<PlanWindow, "width" | "height" | "background" | "transparent">,
	frame: number,
	layers: readonly PlanLayer[],
	output: OutputSize,
): RenderInput {
	const expanded: Json[] = [];
	for (const layer of layers) {
		if (layer.kind === "media") {
			expanded.push(mediaLayer(layer, frame));
			continue;
		}
		const key = bitmapKeyAt(layer, frame);
		if (key !== undefined) expanded.push(bitmapLayer(layer, frame, key));
	}
	const header: Json = {
		frame,
		width: plan.width,
		height: plan.height,
		background: plan.background,
	};
	if (plan.transparent === true) header.transparent = true;
	header.layers = expanded;
	return {
		version: 1,
		output: { width: output.width, height: output.height, scale: output.scale },
		plan: header,
	};
}
