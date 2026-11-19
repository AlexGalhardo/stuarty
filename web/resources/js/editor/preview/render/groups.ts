import type {
	PlanGroupLayer,
	PlanLayer,
	PlanMediaLayer,
	PlanWindowLayer,
} from "../../api/types";
import { isActive, valueAt } from "../planValue";
import { frameInput } from "./frameInput";
import type { RenderSurface } from "./protocol";

// SDD-013 section 3.12: a composition drawn as a group is drawn by the host on
// a surface of its own, with the module, and the module then draws the surface
// as an ordinary media layer. This file expands a group of a plan window into
// the tree the executor draws from (the surface of each group and the layers it
// holds); the geometry is the arithmetic of steps 1 and 2 of that section, which
// the Go host (`compositor.GroupGeometry`) does too. It decides no pixel.

// The largest side of a surface (SDD-013 section 7.2).
export const MAX_SURFACE = 4096;
// The deepest nesting of groups drawn (SDD-001 section 3.5.1).
export const MAX_GROUP_DEPTH = 5;

// Round half up, as the core and the module round (SDD-004 section 6).
function round(value: number): number {
	return Math.floor(value + 0.5);
}

export interface GroupGeometry {
	// The factor from a nested pixel to an output pixel.
	k: number;
	// The surface, in output pixels.
	width: number;
	height: number;
}

// Steps 1 and 2 of SDD-013 section 3.12. `layerW` is the drawn width of the
// group layer in project pixels at the frame, `outScale` the output scale of the
// layer's own frame. Null when the group has nothing to draw.
export function groupGeometry(
	group: { width: number; height: number },
	layerW: number,
	outScale: number,
): GroupGeometry | null {
	if (
		!(group.width >= 1) ||
		!(group.height >= 1) ||
		!(layerW > 0) ||
		!(outScale > 0)
	) {
		return null;
	}
	const k = Math.min(
		(outScale * layerW) / group.width,
		MAX_SURFACE / group.width,
		MAX_SURFACE / group.height,
	);
	return {
		k,
		width: Math.max(round(group.width * k), 1),
		height: Math.max(round(group.height * k), 1),
	};
}

// The byte a layer is drawn with; a layer that rounds to 0 is not drawn and its
// pixels are not fetched (SDD-013 section 3.12).
export function opacityByte(layer: PlanWindowLayer, frame: number): number {
	const opacity = valueAt(layer.opacity, frame, layer.first);
	return round(Math.min(Math.max(opacity, 0), 1) * 255);
}

// Short ids for what the worker holds under an id of at most 64 bytes (SDD-013
// section 4.2): a nested layer's item id is the same in two uses of one
// timeline, and a path of group items can be longer than that. One id for each
// path, the same one from frame to frame.
export class GroupIds {
	private readonly ids = new Map<string, string>();

	of(path: string): string {
		let id = this.ids.get(path);
		if (id === undefined) {
			id = `~${this.ids.size.toString(36)}`;
			this.ids.set(path, id);
		}
		return id;
	}
}

// A layer drawn on its frame, and the scale of that frame: the output scale at
// the top, `k` of the group that holds it inside one.
export interface LeafNode {
	kind: "layer";
	layer: PlanLayer;
	scale: number;
}

export interface GroupNode {
	kind: "group";
	// The group layer with its item and the ids of its layers renamed: `id` is
	// the texture the surface is stored under.
	layer: PlanGroupLayer;
	id: string;
	geometry: GroupGeometry;
	scale: number;
	children: DrawNode[];
}

export type DrawNode = LeafNode | GroupNode;

// The layers present at `frame`, bottom first, with each group expanded. A
// nested layer is named by the id of its path, so two groups never share a
// texture. A group over the depth limit, one with nothing to draw and a layer
// that rounds to no opacity inside a group are left out.
export function expandGroups(
	layers: readonly PlanWindowLayer[],
	frame: number,
	scale: number,
	ids: GroupIds,
	path = "",
	depth = 0,
): DrawNode[] {
	const nodes: DrawNode[] = [];
	for (const layer of layers) {
		if (!isActive(layer, frame)) continue;
		if (
			(depth > 0 || layer.kind === "group") &&
			opacityByte(layer, frame) === 0
		) {
			continue;
		}
		const here = `${path}/${layer.item}`;
		if (layer.kind !== "group") {
			nodes.push({
				kind: "layer",
				layer: depth === 0 ? layer : { ...layer, item: ids.of(here) },
				scale,
			});
			continue;
		}
		if (depth >= MAX_GROUP_DEPTH) continue;
		const geometry = groupGeometry(
			layer.group,
			valueAt(layer.w, frame, layer.first),
			scale,
		);
		if (geometry === null) continue;
		const id = ids.of(here);
		nodes.push({
			kind: "group",
			layer: { ...layer, item: id },
			id,
			geometry,
			scale,
			children: expandGroups(
				layer.group.layers,
				frame,
				geometry.k,
				ids,
				here,
				depth + 1,
			),
		});
	}
	return nodes;
}

// Every layer of a tree that holds pixels of its own, bottom first.
export function leavesOf(nodes: readonly DrawNode[]): PlanLayer[] {
	const leaves: PlanLayer[] = [];
	for (const node of nodes) {
		if (node.kind === "layer") leaves.push(node.layer);
		else leaves.push(...leavesOf(node.children));
	}
	return leaves;
}

// The group layer as the media layer the module draws (SDD-013 section 3.12
// step 4): the surface is its texture, and its crop is the whole nested frame,
// which the plan already says.
export function groupAsMedia(node: GroupNode): PlanMediaLayer {
	const { group: _group, ...layer } = node.layer;
	return {
		...layer,
		kind: "media",
		asset: node.id,
		asset_kind: "image",
		source_seconds: [0],
		color_matrix: null,
	};
}

export interface Drawable {
	// The layers of the frame to draw, bottom first, each group as the media
	// layer of its surface.
	layers: PlanLayer[];
	// The surfaces they need, in the order to draw them: the inner ones first.
	surfaces: RenderSurface[];
}

// The render input of a tree for the layers whose pixels are ready. `ready`
// answers for a layer with pixels of its own and the output scale of its frame
// (what its pixels are delivered at): the layer to draw (it may lose a lookup
// table that is not there yet) or null when the worker holds nothing of it. A
// group none of whose layers is ready draws nothing, so it is left out.
export function drawable(
	nodes: readonly DrawNode[],
	frame: number,
	ready: (layer: PlanLayer, scale: number) => PlanLayer | null,
): Drawable {
	const layers: PlanLayer[] = [];
	const surfaces: RenderSurface[] = [];
	for (const node of nodes) {
		if (node.kind === "layer") {
			const drawn = ready(node.layer, node.scale);
			if (drawn !== null) layers.push(drawn);
			continue;
		}
		const inner = drawable(node.children, frame, ready);
		if (inner.layers.length === 0) continue;
		const { width, height, k } = node.geometry;
		surfaces.push(...inner.surfaces, {
			id: node.id,
			input: frameInput(groupFrame(node), frame, inner.layers, {
				width,
				height,
				scale: k,
			}),
		});
		layers.push(groupAsMedia(node));
	}
	return { layers, surfaces };
}

// The nested frame of a group, as the header of its render input (SDD-013
// section 3.12 step 3): the nested timeline's own background is not used.
export function groupFrame(node: GroupNode): {
	width: number;
	height: number;
	background: string;
	transparent: true;
} {
	return {
		width: node.layer.group.width,
		height: node.layer.group.height,
		background: "#000000",
		transparent: true,
	};
}
