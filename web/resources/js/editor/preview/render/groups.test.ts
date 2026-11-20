import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

import type {
	PlanGroupLayer,
	PlanLayer,
	PlanMediaLayer,
	PlanWindowLayer,
} from "../../api/types";
import {
	drawable,
	expandGroups,
	GroupIds,
	groupGeometry,
	leavesOf,
	MAX_GROUP_DEPTH,
	MAX_SURFACE,
	opacityByte,
} from "./groups";
import type { RenderSurface } from "./protocol";
import { instantiateCore, RenderCore } from "./renderCore";

const moduleBytes = readFileSync(
	new URL("../../../../../../engine/renderhost/renderer.wasm", import.meta.url),
);

function media(item: string, extra: Partial<PlanMediaLayer> = {}): PlanLayer {
	return {
		kind: "media",
		item,
		asset: `ast_${item}`,
		asset_kind: "image",
		first: 0,
		last: 9,
		cx: 4,
		cy: 4,
		w: 8,
		h: 8,
		rotation: 0,
		scale: 1,
		opacity: 1,
		source_seconds: [0],
		crop: [0, 0, 8, 8],
		color_matrix: null,
		...extra,
	};
}

function group(
	item: string,
	layers: PlanWindowLayer[],
	extra: Partial<PlanGroupLayer> = {},
): PlanGroupLayer {
	return {
		kind: "group",
		item,
		first: 0,
		last: 9,
		cx: 4,
		cy: 4,
		w: 8,
		h: 8,
		rotation: 0,
		scale: 1,
		opacity: 1,
		crop: [0, 0, 16, 16],
		group: { width: 16, height: 16, layers },
		...extra,
	};
}

// The expanded layers of a surface's render input.
function surfaceLayers(
	surface: RenderSurface | undefined,
): Record<string, unknown>[] {
	const plan = surface?.input.plan as { layers: Record<string, unknown>[] };
	return plan.layers;
}

describe("groupGeometry (SDD-013 section 3.12 steps 1 and 2)", () => {
	it("scales the nested frame to the drawn size of the group", () => {
		// 16 nested pixels drawn 8 project pixels wide, at output scale 2.
		expect(groupGeometry({ width: 16, height: 8 }, 8, 2)).toEqual({
			k: 1,
			width: 16,
			height: 8,
		});
		expect(groupGeometry({ width: 10, height: 10 }, 5, 0.5)).toEqual({
			k: 0.25,
			// 2.5 rounds up: round half up.
			width: 3,
			height: 3,
		});
	});

	it("keeps a side of the surface at 4096 pixels at most", () => {
		const g = groupGeometry({ width: 1000, height: 4000 }, 8000, 1);
		expect(g?.k).toBe(MAX_SURFACE / 4000);
		expect(g?.height).toBe(MAX_SURFACE);
		expect(g?.width).toBe(1024);
	});

	it("is null for a group with nothing to draw", () => {
		expect(groupGeometry({ width: 0, height: 4 }, 8, 1)).toBeNull();
		expect(groupGeometry({ width: 4, height: 4 }, 0, 1)).toBeNull();
		expect(groupGeometry({ width: 4, height: 4 }, 8, 0)).toBeNull();
	});

	it("never makes a surface of no pixels", () => {
		const g = groupGeometry({ width: 4000, height: 4000 }, 1, 0.001);
		expect(g?.width).toBe(1);
		expect(g?.height).toBe(1);
	});
});

describe("expandGroups", () => {
	it("keeps top-level layers as they are and renames the nested ones by path", () => {
		const ids = new GroupIds();
		const top = media("top");
		const plan: PlanWindowLayer[] = [
			top,
			group("grp", [media("a"), media("b")]),
		];
		const nodes = expandGroups(plan, 3, 1, ids);
		expect(nodes).toHaveLength(2);
		expect(nodes[0]).toMatchObject({ kind: "layer", layer: { item: "top" } });
		const node = nodes[1];
		if (node?.kind !== "group") throw new Error("not a group");
		expect(node.id).toBe(ids.of("/grp"));
		expect(node.layer.item).toBe(node.id);
		expect(
			node.children.map((c) => c.kind === "layer" && c.layer.item),
		).toEqual([ids.of("/grp/a"), ids.of("/grp/b")]);
		// The ids are short and the same from one frame to the next.
		expect(node.id.length).toBeLessThanOrEqual(64);
		const again = expandGroups(plan, 4, 1, ids)[1];
		expect(again?.kind === "group" && again.id).toBe(node.id);
	});

	it("gives the same nested item in two groups two textures", () => {
		const ids = new GroupIds();
		const nodes = expandGroups(
			[group("one", [media("x")]), group("two", [media("x")])],
			0,
			1,
			ids,
		);
		const leaves = leavesOf(nodes).map((l) => l.item);
		expect(new Set(leaves).size).toBe(2);
	});

	it("scales a nested layer by the factor of its group, nested groups by theirs", () => {
		const inner = group("in", [media("m")], {
			w: 4,
			group: { width: 8, height: 8, layers: [media("m")] },
		});
		const outer = group("out", [inner], {
			w: 16,
			group: { width: 16, height: 16, layers: [inner] },
		});
		const nodes = expandGroups([outer], 0, 2, new GroupIds());
		const first = nodes[0];
		if (first?.kind !== "group") throw new Error("not a group");
		// 2 x 16 / 16.
		expect(first.scale).toBe(2);
		expect(first.geometry.k).toBe(2);
		const second = first.children[0];
		if (second?.kind !== "group") throw new Error("not a group");
		// The inner group is drawn 4 wide in the outer frame, at its factor 2.
		expect(second.scale).toBe(2);
		expect(second.geometry).toEqual({ k: 1, width: 8, height: 8 });
	});

	it("leaves out a layer that is not present at the frame", () => {
		const nodes = expandGroups(
			[group("g", [media("early", { last: 2 }), media("late", { first: 5 })])],
			3,
			1,
			new GroupIds(),
		);
		const g = nodes[0];
		expect(g?.kind === "group" && g.children).toEqual([]);
	});

	it("skips a group, and a nested layer, whose opacity rounds to nothing", () => {
		const nodes = expandGroups(
			[
				group("gone", [media("a")], { opacity: 0.001 }),
				group("here", [media("hidden", { opacity: [0, 0.5] }), media("shown")]),
			],
			0,
			1,
			new GroupIds(),
		);
		expect(nodes).toHaveLength(1);
		const here = nodes[0];
		expect(here?.kind === "group" && here.children).toHaveLength(1);
		expect(opacityByte(group("g", []), 0)).toBe(255);
		expect(opacityByte(group("g", [], { opacity: 0.5 }), 0)).toBe(128);
	});

	it("reads the series of a nested layer at the frame of the window", () => {
		const moving = media("m", { first: 4, last: 6, cx: [1, 2, 3] });
		const nodes = expandGroups([group("g", [moving])], 5, 1, new GroupIds());
		const leaf = leavesOf(nodes)[0];
		expect(leaf?.first).toBe(4);
		const out = drawable(nodes, 5, (l) => l);
		expect(surfaceLayers(out.surfaces[0])[0]?.cx).toBe(2);
	});

	it("leaves out a group deeper than the limit", () => {
		let deep: PlanWindowLayer = media("leaf");
		for (let i = 0; i < MAX_GROUP_DEPTH + 1; i++) {
			deep = group(`g${i}`, [deep]);
		}
		const nodes = expandGroups([deep], 0, 1, new GroupIds());
		const reached: number[] = [];
		let level = nodes;
		while (level[0]?.kind === "group") {
			reached.push(reached.length);
			level = level[0].children;
		}
		expect(reached).toHaveLength(MAX_GROUP_DEPTH);
		expect(level).toEqual([]);
	});
});

describe("drawable", () => {
	it("gives the surfaces inner first and the group as a media layer", () => {
		const inner = group("in", [media("m")], {
			group: { width: 8, height: 8, layers: [media("m")] },
		});
		const ids = new GroupIds();
		const nodes = expandGroups(
			[group("out", [inner]), media("top")],
			0,
			1,
			ids,
		);
		const out = drawable(nodes, 0, (l) => l);
		expect(out.surfaces).toHaveLength(2);
		expect(out.surfaces.map((s) => s.id)).toEqual([
			ids.of("/out/in"),
			ids.of("/out"),
		]);
		expect(out.layers.map((l) => l.kind)).toEqual(["media", "media"]);
		const first = out.layers[0];
		expect(first).toMatchObject({
			item: ids.of("/out"),
			asset_kind: "image",
			crop: [0, 0, 16, 16],
		});
		expect(first).not.toHaveProperty("group");
		// The surface is the size of the nested frame at its factor, clear and
		// without the background of the nested timeline.
		expect(out.surfaces[1]?.input).toMatchObject({
			// 16 nested pixels drawn 8 wide.
			output: { width: 8, height: 8, scale: 0.5 },
			plan: { width: 16, height: 16, transparent: true, background: "#000000" },
		});
	});

	it("tells the caller the scale each layer is drawn at: its own frame's", () => {
		const nodes = expandGroups(
			[media("top"), group("g", [media("a")])],
			0,
			2,
			new GroupIds(),
		);
		const seen: [string, number][] = [];
		drawable(nodes, 0, (layer, scale) => {
			seen.push([layer.item.startsWith("~") ? "nested" : layer.item, scale]);
			return layer;
		});
		// The top layer at the output scale; the nested one at 2 x 8 / 16.
		expect(seen).toEqual([
			["top", 2],
			["nested", 1],
		]);
	});

	it("leaves out a group none of whose layers is ready", () => {
		const nodes = expandGroups(
			[group("g", [media("a")])],
			0,
			1,
			new GroupIds(),
		);
		expect(drawable(nodes, 0, () => null)).toEqual({
			layers: [],
			surfaces: [],
		});
	});

	it("passes the layer the caller made ready, not the plan's", () => {
		const nodes = expandGroups(
			[group("g", [media("a", { blur: 3 })])],
			0,
			1,
			new GroupIds(),
		);
		const out = drawable(nodes, 0, (l) =>
			l.kind === "media" ? { ...l, blur: 0 } : l,
		);
		expect(surfaceLayers(out.surfaces[0])[0]).not.toHaveProperty("blur");
	});

	it("is read by the module: a group is drawn as one unit, not as its layers", async () => {
		const core = new RenderCore(await instantiateCore(moduleBytes));
		core.init();
		const solid = (r: number, g: number, b: number): Uint8Array => {
			const px = new Uint8Array(16 * 16 * 4);
			for (let i = 0; i < px.length; i += 4) px.set([r, g, b, 255], i);
			return px;
		};
		// Two opaque layers on top of each other, red under green; the group is 50 %.
		const half = group(
			"g",
			[
				media("red", { w: 16, h: 16, cx: 8, cy: 8 }),
				media("green", { w: 16, h: 16, cx: 8, cy: 8 }),
			],
			{ opacity: 0.5, w: 16, h: 16, cx: 8, cy: 8 },
		);
		const nodes = expandGroups([half], 0, 1, new GroupIds());
		const out = drawable(nodes, 0, (l) => l);
		const [leafRed, leafGreen] = leavesOf(nodes);
		core.putTexture(leafRed?.item ?? "", 16, 16, solid(255, 0, 0), true);
		core.putTexture(leafGreen?.item ?? "", 16, 16, solid(0, 255, 0), true);
		for (const surface of out.surfaces) {
			const { width, height } = surface.input.output;
			const pixels = core.render(JSON.stringify(surface.input), width, height);
			core.putTexture(
				surface.id,
				width,
				height,
				new Uint8Array(pixels.buffer),
				false,
			);
		}
		const frame = {
			version: 1 as const,
			output: { width: 16, height: 16, scale: 1 },
			plan: {
				frame: 0,
				width: 16,
				height: 16,
				background: "#000000",
				layers: out.layers.map((l) => ({
					item: l.item,
					kind: "media",
					asset: "x",
					asset_kind: "image",
					cx: 8,
					cy: 8,
					w: 16,
					h: 16,
					rotation: 0,
					scale: 1,
					opacity: 0.5,
					crop: { x: 0, y: 0, w: 16, h: 16 },
				})),
			},
		};
		const px = core.render(JSON.stringify(frame), 16, 16);
		// Half of the top layer over black: the red under it does not show.
		expect(Array.from(px.slice(0, 4))).toEqual([0, 128, 0, 255]);
	});
});
