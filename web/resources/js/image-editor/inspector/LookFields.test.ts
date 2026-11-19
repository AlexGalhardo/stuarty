import { describe, expect, test } from "vitest";

import type { ImageDocument, Layer, Mask } from "../api/types";
import { maskChange } from "./LookFields";

function doc(mask: Mask | undefined): ImageDocument {
	const box: Layer = {
		id: "lyr_bbbbbbbbbb",
		kind: "shape",
		name: "Box",
		visible: true,
		opacity: 100,
		x: 0,
		y: 0,
		w: 400,
		h: 300,
		mask,
	};
	const group: Layer = {
		id: "lyr_gggggggggg",
		kind: "group",
		name: "Group",
		visible: true,
		opacity: 100,
		x: 0,
		y: 0,
		w: 0,
		h: 0,
		children: [box],
	};
	return {
		image_id: "img_0000000001",
		revision: 3,
		name: "Spec",
		width: 1280,
		height: 720,
		background: "#000000",
		template: false,
		layers: [group],
		findings: [],
		lint: { errors: 0, warnings: 0 },
		text: "",
	};
}

describe("maskChange", () => {
	test("keeps the members of the mask the document holds when the command's turn comes", () => {
		// The render that committed the field saw the mask before the radius and the feather reached the
		// server; the document of the queue has them, and the invert must not send them back to the old values.
		const fresh = doc({
			kind: "rounded_rect",
			w: 600,
			h: 1000,
			radius: 24,
			feather: 6,
		});
		expect(
			maskChange(fresh, "lyr_bbbbbbbbbb", {
				kind: "rounded_rect",
				invert: true,
			}),
		).toEqual([
			{
				op: "layer.mask_set",
				args: {
					layer_id: "lyr_bbbbbbbbbb",
					kind: "rounded_rect",
					w: 600,
					h: 1000,
					radius: 24,
					feather: 6,
					invert: true,
				},
			},
		]);
	});

	test("a layer with no mask sends only what the field gives", () => {
		expect(
			maskChange(doc(undefined), "lyr_bbbbbbbbbb", { kind: "ellipse", w: 500 }),
		).toEqual([
			{
				op: "layer.mask_set",
				args: { layer_id: "lyr_bbbbbbbbbb", kind: "ellipse", w: 500 },
			},
		]);
	});
});
