import type { ImageOperation, Layer } from "../api/types";

// The rows of the layer panel and the requests its gestures make (SDD-012 section 24.5). The panel lists the top
// layer first, the way a person sees the stack; the document lists it last. A request names the layer, its new
// parent and the final position among its new siblings, as `layer.move` takes them; the server decides whether
// the move is allowed.

export interface Row {
	layer: Layer;
	level: number;
	// The group that holds the layer; null at the top level.
	parentId: string | null;
	// The position among its siblings in the document: 0 is the bottom.
	index: number;
	setSize: number;
	// The position among its siblings as listed: 1 is the top one.
	posInSet: number;
	expanded: boolean | null;
}

export function treeRows(
	layers: readonly Layer[],
	collapsed: ReadonlySet<string>,
): Row[] {
	const rows: Row[] = [];
	const visit = (
		list: readonly Layer[],
		level: number,
		parentId: string | null,
	): void => {
		for (let i = list.length - 1; i >= 0; i--) {
			const layer = list[i];
			if (layer === undefined) continue;
			const group = layer.kind === "group";
			rows.push({
				layer,
				level,
				parentId,
				index: i,
				setSize: list.length,
				posInSet: list.length - i,
				expanded: group ? !collapsed.has(layer.id) : null,
			});
			if (group && !collapsed.has(layer.id)) {
				visit(layer.children ?? [], level + 1, layer.id);
			}
		}
	};
	visit(layers, 1, null);
	return rows;
}

export interface Move {
	layerId: string;
	// A group id, or "root" for the top level.
	parent: string;
	index: number;
}

export function moveOp(move: Move): ImageOperation {
	return {
		op: "layer.move",
		args: { layer_id: move.layerId, parent: move.parent, index: move.index },
	};
}

const parentOf = (row: Row): string => row.parentId ?? "root";

// One place up or down the stack among its siblings; null at the end of the stack.
export function step(row: Row, direction: "up" | "down"): Move | null {
	const index = row.index + (direction === "up" ? 1 : -1);
	if (index < 0 || index >= row.setSize) return null;
	return { layerId: row.layer.id, parent: parentOf(row), index };
}

// Out of its group, to the place just above the group; null for a layer at the top level.
export function stepOut(row: Row, rows: readonly Row[]): Move | null {
	if (row.parentId === null) return null;
	const group = rows.find((r) => r.layer.id === row.parentId);
	if (group === undefined) return null;
	return {
		layerId: row.layer.id,
		parent: parentOf(group),
		index: group.index + 1,
	};
}

// Into the group that is directly below it among its siblings, on top of that group; null when there is none.
export function stepIn(row: Row, rows: readonly Row[]): Move | null {
	if (row.index === 0) return null;
	const below = rows.find(
		(r) =>
			r.parentId === row.parentId &&
			r.index === row.index - 1 &&
			r.layer.kind === "group",
	);
	if (below === undefined) return null;
	return {
		layerId: row.layer.id,
		parent: below.layer.id,
		index: below.layer.children?.length ?? 0,
	};
}

export type Drop = "above" | "below" | "inside";

const holds = (group: Layer, id: string): boolean =>
	(group.children ?? []).some((c) => c.id === id || holds(c, id));

// A layer dropped on another: above it, below it or, for a group, inside it. Null when the drop is nothing or
// is into the layer itself or its own subtree.
export function dropMove(
	dragId: string,
	target: Row,
	where: Drop,
	layers: readonly Layer[],
): Move | null {
	if (dragId === target.layer.id) return null;
	const dragged = find(layers, dragId);
	if (dragged === undefined) return null;
	if (dragged.kind === "group" && holds(dragged, target.layer.id)) return null;

	if (where === "inside") {
		if (target.layer.kind !== "group") return null;
		const kids = (target.layer.children ?? []).filter((c) => c.id !== dragId);
		return { layerId: dragId, parent: target.layer.id, index: kids.length };
	}
	const siblings = (
		target.parentId === null
			? layers
			: (find(layers, target.parentId)?.children ?? [])
	).filter((l) => l.id !== dragId);
	const at = siblings.findIndex((l) => l.id === target.layer.id);
	if (at < 0) return null;
	return {
		layerId: dragId,
		parent: parentOf(target),
		index: where === "above" ? at + 1 : at,
	};
}

function find(layers: readonly Layer[], id: string): Layer | undefined {
	for (const layer of layers) {
		if (layer.id === id) return layer;
		const inner = layer.children ? find(layer.children, id) : undefined;
		if (inner) return inner;
	}
	return undefined;
}
