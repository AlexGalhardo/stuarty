import { describe, expect, test } from "vitest";

import {
	adjacentTrack,
	moveToTrackOperations,
	nudgeOperations,
} from "../commands/builders";
import {
	allItemIds,
	clickSelection,
	extendSelection,
	focusAfterRemoval,
	focusTarget,
	type GridLayout,
	gridCommand,
	headerAfterRemoval,
	tabStop,
} from "./keyboard";

const item = (id: string, start: number) => ({ id, start, duration: 10 });

const layout: GridLayout = [
	{ id: "v1", items: [item("a", 0), item("b", 20), item("c", 40)] },
	{ id: "v2", items: [item("d", 18), item("e", 100)] },
	{ id: "a1", items: [] },
	{ id: "v3", items: [item("f", 45)] },
];

const key = (
	name: string,
	mods: Partial<
		Record<"ctrlKey" | "metaKey" | "altKey" | "shiftKey", boolean>
	> = {},
) =>
	gridCommand({
		key: name,
		ctrlKey: false,
		metaKey: false,
		altKey: false,
		shiftKey: false,
		...mods,
	});

describe("gridCommand: the keys of the registry are not grid keys", () => {
	test("slide, roll and slip (VV-036, VV-215) are rows of the registry", () => {
		expect(key("ArrowRight", { ctrlKey: true, altKey: true })).toBeNull();
		expect(key("ArrowLeft", { metaKey: true, altKey: true })).toBeNull();
		expect(
			key("ArrowRight", { ctrlKey: true, altKey: true, shiftKey: true }),
		).toBeNull();
		expect(
			gridCommand({
				key: ",",
				code: "Comma",
				ctrlKey: false,
				metaKey: false,
				altKey: true,
				shiftKey: false,
			}),
		).toBeNull();
	});

	test("Mod+Shift+arrows without Alt stay with the global keys", () => {
		expect(key("ArrowRight", { ctrlKey: true, shiftKey: true })).toBeNull();
	});
});

describe("gridCommand: the key table of section 7.10", () => {
	test("arrows, Home and End move the focus", () => {
		expect(key("ArrowLeft")).toEqual({
			kind: "focus",
			direction: "left",
			extend: false,
		});
		expect(key("ArrowRight")).toEqual({
			kind: "focus",
			direction: "right",
			extend: false,
		});
		expect(key("ArrowUp")).toEqual({
			kind: "focus",
			direction: "up",
			extend: false,
		});
		expect(key("ArrowDown")).toEqual({
			kind: "focus",
			direction: "down",
			extend: false,
		});
		expect(key("Home")).toEqual({
			kind: "focus",
			direction: "home",
			extend: false,
		});
		expect(key("End")).toEqual({
			kind: "focus",
			direction: "end",
			extend: false,
		});
	});

	test("Shift extends along the track only with Left and Right", () => {
		expect(key("ArrowLeft", { shiftKey: true })).toEqual({
			kind: "focus",
			direction: "left",
			extend: true,
		});
		expect(key("ArrowRight", { shiftKey: true })).toMatchObject({
			extend: true,
		});
		expect(key("ArrowUp", { shiftKey: true })).toBeNull();
	});

	test("Enter activates, Space is never taken", () => {
		expect(key("Enter")).toEqual({ kind: "activate" });
		expect(key(" ")).toBeNull();
	});

	test("Alt nudges by a frame, Shift+Alt by a second, Alt+Up and Down change track", () => {
		expect(key("ArrowLeft", { altKey: true })).toEqual({
			kind: "nudge",
			unit: "frame",
			sign: -1,
		});
		expect(key("ArrowRight", { altKey: true })).toEqual({
			kind: "nudge",
			unit: "frame",
			sign: 1,
		});
		expect(key("ArrowRight", { altKey: true, shiftKey: true })).toEqual({
			kind: "nudge",
			unit: "second",
			sign: 1,
		});
		expect(key("ArrowUp", { altKey: true })).toEqual({
			kind: "move-track",
			sign: -1,
		});
		expect(key("ArrowDown", { altKey: true })).toEqual({
			kind: "move-track",
			sign: 1,
		});
	});

	test("Mod+A and the other Mod keys are the registry's, not the grid's", () => {
		expect(key("a", { ctrlKey: true })).toBeNull();
		expect(key("A", { metaKey: true })).toBeNull();
		expect(key("ArrowLeft", { ctrlKey: true })).toBeNull();
		expect(key("s")).toBeNull();
	});
});

describe("focusTarget", () => {
	test("Left and Right walk the track and stop at its ends", () => {
		expect(focusTarget(layout, "b", "left")).toBe("a");
		expect(focusTarget(layout, "b", "right")).toBe("c");
		expect(focusTarget(layout, "a", "left")).toBeNull();
		expect(focusTarget(layout, "c", "right")).toBeNull();
	});

	test("Home and End go to the first and last item of the track", () => {
		expect(focusTarget(layout, "b", "home")).toBe("a");
		expect(focusTarget(layout, "b", "end")).toBe("c");
	});

	test("Up and Down land on the nearest item by start, skipping empty tracks", () => {
		expect(focusTarget(layout, "b", "down")).toBe("d");
		expect(focusTarget(layout, "c", "down")).toBe("d");
		expect(focusTarget(layout, "e", "down")).toBe("f");
		expect(focusTarget(layout, "d", "up")).toBe("b");
		expect(focusTarget(layout, "a", "up")).toBeNull();
		expect(focusTarget(layout, "f", "down")).toBeNull();
	});

	test("with no focus the first item is the target", () => {
		expect(focusTarget(layout, null, "right")).toBe("a");
		expect(focusTarget(layout, "gone", "right")).toBe("a");
	});
});

describe("focus is never lost when an item is removed", () => {
	const without = (id: string): GridLayout =>
		layout.map((t) => ({ ...t, items: t.items.filter((i) => i.id !== id) }));

	test("the next item on the track, else the previous one", () => {
		expect(focusAfterRemoval(layout, "b", without("b"))).toBe("c");
		expect(focusAfterRemoval(layout, "c", without("c"))).toBe("b");
	});

	test("when the track is emptied, the first item anywhere", () => {
		const next = layout.map((t) => (t.id === "v3" ? { ...t, items: [] } : t));
		expect(focusAfterRemoval(layout, "f", next)).toBe("a");
	});

	test("a removal that takes the neighbour too skips to a survivor", () => {
		const next = layout.map((t) =>
			t.id === "v1" ? { ...t, items: t.items.filter((i) => i.id === "c") } : t,
		);
		expect(focusAfterRemoval(layout, "a", next)).toBe("c");
	});

	test("the header of the track takes the focus when its last item left", () => {
		const next = layout.map((t) => (t.id === "v3" ? { ...t, items: [] } : t));
		expect(headerAfterRemoval(layout, "f", next)).toBe("v3");
		expect(headerAfterRemoval(layout, "b", without("b"))).toBeNull();
		expect(
			headerAfterRemoval(
				layout,
				"f",
				layout.filter((t) => t.id !== "v3"),
			),
		).toBeNull();
	});

	test("an empty timeline leaves no target", () => {
		expect(
			focusAfterRemoval(layout, "a", [{ id: "v1", items: [] }]),
		).toBeNull();
	});

	test("the tab stop falls back to the first item", () => {
		expect(tabStop(layout, "e")).toBe("e");
		expect(tabStop(layout, "gone")).toBe("a");
		expect(tabStop(layout, null)).toBe("a");
		expect(tabStop([], null)).toBeNull();
	});
});

describe("selection", () => {
	const none = { shift: false, toggle: false };

	test("a click selects one item and anchors there", () => {
		expect(clickSelection(layout, ["a"], "a", "b", none)).toEqual({
			items: ["b"],
			anchor: "b",
		});
	});

	test("Shift+click selects the range on one track from the anchor", () => {
		expect(
			clickSelection(layout, ["a"], "a", "c", { shift: true, toggle: false }),
		).toEqual({
			items: ["a", "b", "c"],
			anchor: "a",
		});
		expect(
			clickSelection(layout, ["c"], "c", "a", { shift: true, toggle: false })
				.items,
		).toEqual(["a", "b", "c"]);
	});

	test("Shift+click across tracks selects the clicked item alone", () => {
		expect(
			clickSelection(layout, ["a"], "a", "d", { shift: true, toggle: false }),
		).toEqual({
			items: ["d"],
			anchor: "d",
		});
	});

	test("Ctrl or Cmd+click toggles", () => {
		const toggle = { shift: false, toggle: true };
		expect(clickSelection(layout, ["a"], "a", "b", toggle).items).toEqual([
			"a",
			"b",
		]);
		expect(clickSelection(layout, ["a", "b"], "b", "a", toggle).items).toEqual([
			"b",
		]);
	});

	test("Shift+Arrow extends from the anchor to the new focus", () => {
		expect(extendSelection(layout, "a", "b")).toEqual({
			items: ["a", "b"],
			anchor: "a",
		});
		expect(extendSelection(layout, null, "b")).toEqual({
			items: ["b"],
			anchor: "b",
		});
	});

	test("select all lists every item of every track", () => {
		expect(allItemIds(layout)).toEqual(["a", "b", "c", "d", "e", "f"]);
	});
});

describe("the nudge and the track move are one batch", () => {
	test("one item.move per selected item, never below frame 0", () => {
		const ops = nudgeOperations(
			[
				{ id: "a", start: 0, duration: 10 },
				{ id: "b", start: 20, duration: 10 },
				{ id: "c", start: 40, duration: 10 },
			],
			-1,
		);
		expect(ops).toEqual([
			{ op: "item.move", args: { item_id: "a", start: "0f" } },
			{ op: "item.move", args: { item_id: "b", start: "19f" } },
			{ op: "item.move", args: { item_id: "c", start: "39f" } },
		]);
	});

	test("moving right, the last item goes first so neighbours do not collide", () => {
		const ops = nudgeOperations(
			[
				{ id: "a", start: 0, duration: 10 },
				{ id: "b", start: 10, duration: 10 },
			],
			1,
		);
		expect(ops.map((o) => o.args.item_id)).toEqual(["b", "a"]);
	});

	test("a second is the project frame rate in frames", () => {
		expect(
			nudgeOperations([{ id: "a", start: 10, duration: 5 }], 30)[0]?.args,
		).toEqual({
			item_id: "a",
			start: "40f",
		});
	});

	test("the track above or below, none past the ends", () => {
		const ids = ["v1", "v2", "a1"];
		expect(adjacentTrack(ids, "v2", -1)).toBe("v1");
		expect(adjacentTrack(ids, "v2", 1)).toBe("a1");
		expect(adjacentTrack(ids, "v1", -1)).toBeNull();
		expect(adjacentTrack(ids, "a1", 1)).toBeNull();
		expect(moveToTrackOperations([{ itemId: "x", trackId: "v1" }])).toEqual([
			{ op: "item.move", args: { item_id: "x", track_id: "v1" } },
		]);
	});
});
