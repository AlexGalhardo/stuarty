import type { Layer, LayerKind } from "../api/types";

// The boxes of the layers in canvas pixels, for the outline, the hit test and the snapping (SDD-012 section
// 24.4). The offsets of the groups are added so a child is where the plan draws it; this is display arithmetic
// over the read model, and the server remains the one that decides where a layer is.

export interface LayerBox {
	id: string;
	kind: LayerKind;
	name: string;
	x: number;
	y: number;
	w: number;
	h: number;
	rotation: number;
	// False when the layer or one of its groups is hidden.
	visible: boolean;
	// True when the layer or one of its groups is locked.
	locked: boolean;
	// What the groups above add to the x and y of the layer to give the canvas position.
	offsetX: number;
	offsetY: number;
	depth: number;
}

// Every layer, bottom first, the way the plan stacks them. The box of a group is the union of its children.
export function layerBoxes(layers: readonly Layer[]): LayerBox[] {
	const out: LayerBox[] = [];
	const visit = (
		list: readonly Layer[],
		ox: number,
		oy: number,
		depth: number,
		visible: boolean,
		locked: boolean,
	): void => {
		for (const layer of list) {
			const shown = visible && layer.visible;
			const frozen = locked || layer.locked === true;
			if (layer.kind !== "group") {
				out.push({
					id: layer.id,
					kind: layer.kind,
					name: layer.name,
					x: layer.x + ox,
					y: layer.y + oy,
					w: layer.w,
					h: layer.h,
					rotation: layer.rotation ?? 0,
					visible: shown,
					locked: frozen,
					offsetX: ox,
					offsetY: oy,
					depth,
				});
				continue;
			}
			const at = out.length;
			out.push({
				id: layer.id,
				kind: "group",
				name: layer.name,
				x: 0,
				y: 0,
				w: 0,
				h: 0,
				rotation: 0,
				visible: shown,
				locked: frozen,
				offsetX: ox,
				offsetY: oy,
				depth,
			});
			visit(
				layer.children ?? [],
				ox + layer.x,
				oy + layer.y,
				depth + 1,
				shown,
				frozen,
			);
			const kids = out.slice(at + 1).filter((b) => b.kind !== "group");
			const group = out[at];
			if (group !== undefined && kids.length > 0) {
				const left = Math.min(...kids.map((b) => b.x));
				const top = Math.min(...kids.map((b) => b.y));
				const right = Math.max(...kids.map((b) => b.x + b.w));
				const bottom = Math.max(...kids.map((b) => b.y + b.h));
				group.x = left;
				group.y = top;
				group.w = right - left;
				group.h = bottom - top;
			}
		}
	};
	visit(layers, 0, 0, 0, true, false);
	return out;
}
