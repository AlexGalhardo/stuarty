import type { ImageDocument, ImageOperation, Layer } from "../api/types";
import type { ImageActions } from "../state/store";
import { newLayerId } from "./units";

// Where a layer is: its group (undefined at the top level) and its position among its siblings.
function locate(
	layers: readonly Layer[],
	id: string,
	parent?: string,
): { parent: string | undefined; index: number } | null {
	for (let i = 0; i < layers.length; i++) {
		const layer = layers[i];
		if (layer === undefined) continue;
		if (layer.id === id) return { parent, index: i };
		const inner = layer.children ? locate(layer.children, id, layer.id) : null;
		if (inner !== null) return inner;
	}
	return null;
}

// The operations that swap the photo of an image layer: the new image goes in just above the old one, covers
// its box, takes its name, slot and opacity, and the old one is removed, all in one batch so it is one step of
// the history. The catalogue has no operation that changes the file of a layer, and this uses only those it has.
export function replaceOps(
	doc: ImageDocument,
	layer: Layer,
	hash: string,
): ImageOperation[] {
	const at = locate(doc.layers, layer.id);
	if (at === null) return [];
	const id = newLayerId();
	const ops: ImageOperation[] = [
		{
			op: "layer.add_image",
			args: {
				asset: hash,
				id,
				name: layer.name,
				...(at.parent === undefined ? {} : { parent: at.parent }),
				index: at.index + 1,
			},
		},
		{ op: "layer.fit", args: { layer_id: id, mode: "cover", to: layer.id } },
		{ op: "layer.remove", args: { layer_id: layer.id } },
	];
	const keep: { [key: string]: unknown } = { layer_id: id };
	if (layer.slot !== undefined && layer.slot !== "") keep.slot = layer.slot;
	if (layer.opacity !== 100) keep.opacity = layer.opacity;
	if (Object.keys(keep).length > 1) ops.push({ op: "layer.set", args: keep });
	if ((layer.rotation ?? 0) !== 0) {
		ops.push({
			op: "layer.transform",
			args: { layer_id: id, rotation: layer.rotation },
		});
	}
	return ops;
}

export function replacePhoto(
	send: ImageActions["send"],
	doc: ImageDocument,
	layer: Layer,
	hash: string,
): Promise<unknown> {
	const ops = replaceOps(doc, layer, hash);
	const made = ops[0]?.args?.id;
	return send(ops, () => (typeof made === "string" ? [made] : undefined));
}
