import { describe, expect, test } from "vitest";

import type { Layer } from "../api/types";
import { lockClass, rowClass } from "./rowStyle";
import { countLayers } from "./tree";

const layer = (id: string, children?: Layer[]): Layer => ({
	id,
	kind: children === undefined ? "shape" : "group",
	name: id,
	visible: true,
	opacity: 100,
	x: 0,
	y: 0,
	w: 10,
	h: 10,
	children,
});

describe("layers panel row", () => {
	test("a selected row has a bar at its start edge, so selection is not colour alone", () => {
		const selected = rowClass({ selected: true, visible: true, mark: null });
		expect(selected).toContain("inset_2px_0_0");
		expect(
			rowClass({ selected: false, visible: true, mark: null }),
		).not.toContain("inset_2px_0_0");
	});

	test("a hidden layer is dimmed and a drop shows its mark", () => {
		expect(rowClass({ selected: false, visible: false, mark: null })).toContain(
			"opacity-60",
		);
		expect(
			rowClass({ selected: false, visible: true, mark: "above" }),
		).toContain("border-t-2");
		expect(
			rowClass({ selected: false, visible: true, mark: "below" }),
		).toContain("border-b-2");
		expect(
			rowClass({ selected: false, visible: true, mark: "inside" }),
		).toContain("ring-2");
	});

	test("an open padlock shows only on hover, focus or selection; a closed one always", () => {
		expect(lockClass(false)).toContain("opacity-0");
		expect(lockClass(false)).toContain("group-hover:opacity-70");
		expect(lockClass(true)).not.toContain("opacity-0");
	});
});

describe("countLayers", () => {
	test("counts a group and each of its children once, at any depth", () => {
		expect(countLayers([])).toBe(0);
		expect(
			countLayers([
				layer("a"),
				layer("g", [layer("c1"), layer("h", [layer("c2")])]),
			]),
		).toBe(5);
	});
});
