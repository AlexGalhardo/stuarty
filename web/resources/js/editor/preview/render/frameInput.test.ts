import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

import type { PlanBitmapLayer, PlanMediaLayer } from "../../api/types";
import { frameInput } from "./frameInput";
import { instantiateCore, RenderCore, RenderError } from "./renderCore";

const moduleBytes = readFileSync(
	new URL("../../../../../../engine/renderhost/renderer.wasm", import.meta.url),
);

const head = {
	width: 8,
	height: 8,
	background: "#102030",
} as const;

function media(extra: Partial<PlanMediaLayer> = {}): PlanMediaLayer {
	return {
		kind: "media",
		item: "itm_a",
		asset: "ast_a",
		asset_kind: "video",
		first: 10,
		last: 12,
		cx: [4, 5, 6],
		cy: 4,
		w: 8,
		h: 8,
		rotation: 0,
		scale: 1,
		opacity: [1, 0.5, 0.25],
		source_seconds: [0, 0.04, 0.08],
		crop: [0, 0, 4, 4],
		color_matrix: null,
		...extra,
	};
}

const output = { width: 8, height: 8, scale: 1 };

describe("frameInput (SDD-013 section 2.4)", () => {
	it("takes the element of a per-frame series at frame minus first", () => {
		const out = frameInput(head, 11, [media()], output);
		const layer = (out.plan as { layers: Record<string, unknown>[] }).layers[0];
		expect(layer).toMatchObject({
			item: "itm_a",
			kind: "media",
			cx: 5,
			cy: 4,
			opacity: 0.5,
			crop: { x: 0, y: 0, w: 4, h: 4 },
		});
		expect(layer).not.toHaveProperty("source_seconds");
		expect(layer).not.toHaveProperty("color_matrix");
		expect(layer).not.toHaveProperty("blend");
	});

	it("keeps plan order, the background and the transparent flag", () => {
		const a = media({ item: "low" });
		const b = media({ item: "high", blend: "overlay" });
		const out = frameInput(
			{ ...head, transparent: true },
			10,
			[a, b],
			output,
		) as { plan: { layers: { item: string; blend?: string }[] } };
		expect(out.plan.layers.map((l) => l.item)).toEqual(["low", "high"]);
		expect(out.plan.layers[1]?.blend).toBe("overlay");
		expect(out.plan).toMatchObject({
			background: "#102030",
			transparent: true,
			frame: 10,
		});
	});

	it("uses the key the window gives the frame for a bitmap layer", () => {
		const text: PlanBitmapLayer = {
			kind: "bitmap",
			item: "itm_t",
			first: 0,
			last: 3,
			cx: 4,
			cy: 4,
			w: 6,
			h: 2,
			rotation: 0,
			scale: 1,
			opacity: 1,
			bitmaps: { keys: ["k0", "k1"], index: [0, 0, 1, 1] },
			natural: [6, 2],
		};
		const out = frameInput(head, 2, [text], output) as {
			plan: { layers: Record<string, unknown>[] };
		};
		expect(out.plan.layers[0]).toMatchObject({
			kind: "bitmap",
			bitmap_key: "k1",
			bitmap: { kind: "text" },
		});
	});

	it("is a render input the module reads, for a layer that uses every field", async () => {
		const core = new RenderCore(await instantiateCore(moduleBytes));
		core.init();
		core.putTexture("itm_a", 2, 2, new Uint8Array(16).fill(200), true);
		const layer = media({
			blend: "overlay",
			mask: { shape: "ellipse", x: 0, y: 0, width: 1, height: 1, feather: 2 },
			flip_x: true,
			effects: [{ kind: "vignette", amount: 0.3, softness: 0.4 }],
			transition: [null, { kind: "dip", edge: "in", p: 20000 }, null],
			shadow: { color: "#000000", opacity: 0.5, dx: 1, dy: 1, blur: 1 },
			outline: { color: "#ffffff", width: 1 },
			chroma_key: {
				color: "#66DD09",
				similarity: 0.3,
				smoothness: 0.1,
				spill: 0.1,
			},
			color_matrix: [1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0],
			filter: [1, 0, 0, 0, 0, 1, 0, 0, 0, 0, 1, 0],
			blur: 1,
			grade: { wheels: { shadows: { hue: 10, amount: 0.1, luma: 0 } } },
		});
		const out = frameInput(head, 11, [layer], output);
		const pixels = core.render(JSON.stringify(out), 8, 8);
		expect(pixels).toHaveLength(8 * 8 * 4);
	});

	it("leaves out a layer the module would refuse, never sending a field it does not know", async () => {
		const core = new RenderCore(await instantiateCore(moduleBytes));
		core.init();
		const out = frameInput(head, 10, [media()], output);
		// No texture was uploaded: the refusal names the item, not the plan.
		expect(() => core.render(JSON.stringify(out), 8, 8)).toThrow(RenderError);
		try {
			core.render(JSON.stringify(out), 8, 8);
		} catch (err) {
			expect(err).toMatchObject({ reason: "missing_texture", detail: "itm_a" });
		}
	});
});
