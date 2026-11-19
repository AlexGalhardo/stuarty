import { describe, expect, test } from "vitest";

import type { Layer } from "../api/types";
import { dropMove, step, stepIn, stepOut, treeRows } from "./tree";

const layer = (id: string, extra: Partial<Layer> = {}): Layer => ({
	id,
	kind: "shape",
	name: id,
	visible: true,
	opacity: 100,
	x: 0,
	y: 0,
	w: 10,
	h: 10,
	...extra,
});

// bottom to top: a, G(c1, c2), b
const layers: Layer[] = [
	layer("a"),
	layer("G", { kind: "group", children: [layer("c1"), layer("c2")] }),
	layer("b"),
];

describe("treeRows", () => {
	test("lists the top layer first, a group's children under it", () => {
		const rows = treeRows(layers, new Set());
		expect(rows.map((r) => [r.layer.id, r.level])).toEqual([
			["b", 1],
			["G", 1],
			["c2", 2],
			["c1", 2],
			["a", 1],
		]);
	});

	test("carries the positions the tree roles need", () => {
		const rows = treeRows(layers, new Set());
		const g = rows.find((r) => r.layer.id === "G");
		expect(g).toMatchObject({
			posInSet: 2,
			setSize: 3,
			index: 1,
			expanded: true,
		});
		expect(rows.find((r) => r.layer.id === "c1")).toMatchObject({
			posInSet: 2,
			setSize: 2,
			parentId: "G",
		});
		expect(rows.find((r) => r.layer.id === "a")?.expanded).toBeNull();
	});

	test("a collapsed group hides its children", () => {
		const rows = treeRows(layers, new Set(["G"]));
		expect(rows.map((r) => r.layer.id)).toEqual(["b", "G", "a"]);
		expect(rows[1]?.expanded).toBe(false);
	});
});

describe("keyboard moves", () => {
	const rows = treeRows(layers, new Set());
	const row = (id: string) => {
		const r = rows.find((x) => x.layer.id === id);
		if (!r) throw new Error(id);
		return r;
	};

	test("up and down are one place among the siblings, and stop at the ends", () => {
		expect(step(row("a"), "up")).toEqual({
			layerId: "a",
			parent: "root",
			index: 1,
		});
		expect(step(row("b"), "up")).toBeNull();
		expect(step(row("a"), "down")).toBeNull();
		expect(step(row("c1"), "up")).toEqual({
			layerId: "c1",
			parent: "G",
			index: 1,
		});
	});

	test("out goes to the place just above the group", () => {
		expect(stepOut(row("c1"), rows)).toEqual({
			layerId: "c1",
			parent: "root",
			index: 2,
		});
		expect(stepOut(row("a"), rows)).toBeNull();
	});

	test("in goes on top of the group directly below, and only then", () => {
		expect(stepIn(row("b"), rows)).toEqual({
			layerId: "b",
			parent: "G",
			index: 2,
		});
		expect(stepIn(row("a"), rows)).toBeNull();
		expect(stepIn(row("G"), rows)).toBeNull();
	});
});

describe("drop", () => {
	const rows = treeRows(layers, new Set());
	const row = (id: string) => {
		const r = rows.find((x) => x.layer.id === id);
		if (!r) throw new Error(id);
		return r;
	};

	test("above and below a sibling give the final position among the new siblings", () => {
		// drag a above b: it ends on top
		expect(dropMove("a", row("b"), "above", layers)).toEqual({
			layerId: "a",
			parent: "root",
			index: 2,
		});
		// drag b below a: it ends at the bottom
		expect(dropMove("b", row("a"), "below", layers)).toEqual({
			layerId: "b",
			parent: "root",
			index: 0,
		});
	});

	test("a drop inside a group goes on top of it; into anything else is nothing", () => {
		expect(dropMove("a", row("G"), "inside", layers)).toEqual({
			layerId: "a",
			parent: "G",
			index: 2,
		});
		expect(dropMove("a", row("b"), "inside", layers)).toBeNull();
	});

	test("a group cannot be dropped into its own subtree, nor a layer on itself", () => {
		expect(dropMove("G", row("c1"), "above", layers)).toBeNull();
		expect(dropMove("a", row("a"), "above", layers)).toBeNull();
	});

	test("a drop next to a child names its group as the parent", () => {
		expect(dropMove("a", row("c1"), "below", layers)).toEqual({
			layerId: "a",
			parent: "G",
			index: 0,
		});
	});
});
