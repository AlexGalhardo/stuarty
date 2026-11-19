import { afterEach, describe, expect, test, vi } from "vitest";

import { readKeymap } from "../state/prefs";
import { PRESET_IDS, rowById, SHORTCUTS } from "./keymap";
import {
	findConflict,
	getKeymap,
	isRowKey,
	keyLabel,
	keysOf,
	labelWithKey,
	rebind,
	resetBindings,
	rowFor,
	setPreset,
} from "./keymapStore";

const press = (
	key: string,
	mods: Partial<
		Record<"ctrlKey" | "metaKey" | "altKey" | "shiftKey", boolean>
	> & { code?: string } = {},
) => ({
	key,
	ctrlKey: false,
	metaKey: false,
	altKey: false,
	shiftKey: false,
	...mods,
});

function stubStorage(
	initial: Record<string, string> = {},
): Map<string, string> {
	const data = new Map<string, string>(Object.entries(initial));
	vi.stubGlobal("localStorage", {
		getItem: (k: string) => data.get(k) ?? null,
		setItem: (k: string, v: string) => void data.set(k, v),
		removeItem: (k: string) => void data.delete(k),
	});
	return data;
}

afterEach(() => {
	vi.unstubAllGlobals();
	setPreset("short");
	resetBindings();
});

describe("rowFor", () => {
	test("names the row of a key press in the scopes asked for", () => {
		expect(rowFor(press("b", { ctrlKey: true }), ["timeline"])).toBe("split");
		expect(rowFor(press("b", { metaKey: true }), ["timeline"])).toBe("split");
		expect(rowFor(press(" "), ["preview"])).toBe("play_pause");
		expect(rowFor(press("k", { ctrlKey: true }), ["text"])).toBe("palette");
		expect(rowFor(press("z", { ctrlKey: true }), ["global"])).toBe("undo");
	});

	test("a row of another scope does not answer", () => {
		expect(rowFor(press(" "), ["timeline", "global"])).toBeNull();
		expect(rowFor(press("b", { ctrlKey: true }), ["preview"])).toBeNull();
	});

	test("an unknown key is no row", () => {
		expect(
			rowFor(press("F9"), ["global", "timeline", "preview", "text"]),
		).toBeNull();
	});

	test("the aliases and the second keys answer", () => {
		expect(rowFor(press("+", { shiftKey: true }), ["timeline"])).toBe(
			"zoom_in",
		);
		expect(rowFor(press("y", { ctrlKey: true }), ["global"])).toBe("redo");
		expect(rowFor(press("Backspace"), ["timeline"])).toBe("delete");
		expect(rowFor(press("PageUp"), ["timeline"])).toBe("step_second_forward");
	});

	test("the slip keys are read by the physical key", () => {
		expect(
			rowFor(press("˙", { altKey: true, code: "Period" }), ["timeline"]),
		).toBe("slip_forward");
		expect(
			rowFor(press(",", { altKey: true, code: "Comma" }), ["timeline"]),
		).toBe("slip_back");
	});
});

describe("presets", () => {
	test("the classic preset moves the rows the table lists and keeps the others", () => {
		expect(rowFor(press("s"), ["timeline"])).toBeNull();
		setPreset("classic");
		expect(getKeymap().preset).toBe("classic");
		expect(rowFor(press("s"), ["timeline"])).toBe("split");
		expect(rowFor(press("b", { ctrlKey: true }), ["timeline"])).toBeNull();
		expect(rowFor(press("c"), ["timeline"])).toBe("tool_blade");
		expect(rowFor(press("v"), ["timeline"])).toBe("tool_select");
		expect(rowFor(press("C", { shiftKey: true }), ["timeline"])).toBe(
			"add_caption",
		);
		// Unchanged rows.
		expect(rowFor(press("q"), ["timeline"])).toBe("trim_start");
		expect(rowFor(press("z", { ctrlKey: true }), ["global"])).toBe("undo");
	});

	test("every row has a key in every preset", () => {
		for (const preset of PRESET_IDS) {
			setPreset(preset);
			for (const row of SHORTCUTS) {
				if (row.fixed === true) continue;
				expect(keysOf(row).length, `${row.id} in ${preset}`).toBeGreaterThan(0);
			}
		}
	});

	test("a change of preset drops the override that collides with the new keys", () => {
		expect(rebind("trim_end", "S")).toEqual({ ok: true });
		const result = setPreset("classic");
		expect(result.dropped).toEqual(["trim_end"]);
		expect(rowFor(press("s"), ["timeline"])).toBe("split");
		expect(rowFor(press("w"), ["timeline"])).toBe("trim_end");
	});

	test("an override that does not collide survives a change of preset", () => {
		expect(rebind("trim_end", "X")).toEqual({ ok: true });
		expect(setPreset("classic").dropped).toEqual([]);
		expect(rowFor(press("x"), ["timeline"])).toBe("trim_end");
	});
});

describe("rebinding", () => {
	test("a free key takes the row and the old key stops matching", () => {
		expect(rebind("split", "X")).toEqual({ ok: true });
		expect(rowFor(press("x"), ["timeline"])).toBe("split");
		expect(rowFor(press("b", { ctrlKey: true }), ["timeline"])).toBeNull();
		expect(isRowKey(press("x"), "split")).toBe(true);
	});

	test("every row that is not fixed can be rebound to a free key", () => {
		for (const row of SHORTCUTS) {
			if (row.fixed === true) continue;
			expect(rebind(row.id, "Mod+Alt+9"), row.id).toEqual({ ok: true });
			expect(rebind(row.id, "Mod+Alt+8"), row.id).toEqual({ ok: true });
			resetBindings();
		}
	});

	test("a key of another row is refused and names that row", () => {
		expect(rebind("split", "C")).toEqual({
			ok: false,
			conflict: "add_caption",
		});
		expect(rebind("split", "Mod+Z")).toEqual({ ok: false, conflict: "undo" });
		expect(rebind("undo", "Mod+B")).toEqual({ ok: false, conflict: "split" });
		expect(rebind("add_text", "Mod+Shift+B")).toEqual({
			ok: false,
			conflict: "split_all",
		});
		expect(rebind("add_text", "[")).toEqual({
			ok: false,
			conflict: "select_left",
		});
		const split = rowById("split");
		expect(split === undefined ? [] : keysOf(split)).toEqual(["Mod+B"]);
	});

	test("a global row conflicts with a key of any scope", () => {
		// Mod+E is `export` (global), so a timeline row cannot take it.
		expect(findConflict("add_text", "Mod+E")).toBe("export");
		// Esc, Tab and the keys of the browser are reserved.
		expect(findConflict("split", "Tab")).toBe("reserved");
		expect(findConflict("split", "Esc")).toBe("reserved");
		expect(findConflict("split", "Mod+W")).toBe("reserved");
	});

	test("two rows of scopes that never overlap may share a key", () => {
		expect(findConflict("play_pause", "Left")).toBeNull();
		expect(findConflict("step_back", "Space")).toBeNull();
	});

	test("a row never conflicts with its own key or its own aliases", () => {
		expect(findConflict("zoom_in", "+")).toBeNull();
		expect(findConflict("add_text", "+")).toBe("zoom_in");
		expect(findConflict("split", "Mod+B")).toBeNull();
	});

	test("a key freed by a rebinding can be taken by another row", () => {
		expect(rebind("add_caption", "Mod+Alt+9")).toEqual({ ok: true });
		expect(rebind("add_text", "C")).toEqual({ ok: true });
		expect(rowFor(press("c"), ["timeline"])).toBe("add_text");
	});

	test("a fixed row cannot be rebound", () => {
		expect(rebind("cancel", "X")).toEqual({ ok: false, conflict: "reserved" });
	});

	test("a rebound zoom row drops its aliases", () => {
		rebind("zoom_in", "X");
		expect(rowFor(press("+", { shiftKey: true }), ["timeline"])).toBeNull();
		expect(rowFor(press("x"), ["timeline"])).toBe("zoom_in");
	});
});

describe("storage", () => {
	test("only the preset and the overrides are stored, and reset clears the overrides", () => {
		const data = stubStorage();
		rebind("split", "X");
		expect(JSON.parse(data.get("vv.keymap") ?? "null")).toEqual({
			preset: null,
			overrides: { split: "X" },
		});
		setPreset("classic");
		expect(JSON.parse(data.get("vv.keymap") ?? "null")).toEqual({
			preset: "classic",
			overrides: { split: "X" },
		});
		resetBindings();
		expect(JSON.parse(data.get("vv.keymap") ?? "null")).toEqual({
			preset: "classic",
			overrides: {},
		});
		setPreset("short");
		expect(data.has("vv.keymap")).toBe(false);
	});

	test("a key that is the preset's own is no override", () => {
		const data = stubStorage();
		rebind("split", "X");
		rebind("split", "Mod+B");
		expect(data.has("vv.keymap")).toBe(false);
	});

	test("reads the new format and the flat format of the first release", () => {
		stubStorage({
			"vv.keymap": JSON.stringify({
				preset: "classic",
				overrides: { split: "X" },
			}),
		});
		expect(readKeymap()).toEqual({
			preset: "classic",
			overrides: { split: "X" },
		});
		stubStorage({ "vv.keymap": JSON.stringify({ split: "X" }) });
		expect(readKeymap()).toEqual({ preset: null, overrides: { split: "X" } });
		stubStorage({ "vv.keymap": "not json" });
		expect(readKeymap()).toEqual({ preset: null, overrides: {} });
		stubStorage({ "vv.keymap": "[1]" });
		expect(readKeymap()).toEqual({ preset: null, overrides: {} });
	});
});

describe("labelWithKey", () => {
	test("adds the first key of the row, Mod by platform", () => {
		expect(labelWithKey("Split", "split", getKeymap(), false)).toBe(
			"Split (Ctrl+B)",
		);
		expect(labelWithKey("Split", "split", getKeymap(), true)).toBe(
			"Split (Cmd+B)",
		);
		expect(labelWithKey("Delete", "delete", getKeymap(), false)).toBe(
			"Delete (Delete)",
		);
	});

	test("follows a rebound key and the preset", () => {
		rebind("split", "X");
		expect(labelWithKey("Split", "split", getKeymap(), false)).toBe(
			"Split (X)",
		);
		resetBindings();
		setPreset("classic");
		expect(keyLabel("split", getKeymap(), false)).toBe("S");
	});

	test("leaves the label alone for an unknown row", () => {
		expect(labelWithKey("Fit", "nope", getKeymap(), false)).toBe("Fit");
	});
});
