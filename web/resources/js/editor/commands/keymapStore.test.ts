import { afterEach, describe, expect, test, vi } from "vitest";

import { comboFromEvent, DEFAULT_BINDINGS } from "./keymap";
import {
	bindingFor,
	findConflict,
	getBindings,
	rebind,
	resetBindings,
} from "./keymapStore";

const press = (key: string, mods: Partial<KeyboardEvent> = {}) => ({
	key,
	ctrlKey: false,
	metaKey: false,
	altKey: false,
	shiftKey: false,
	...mods,
});

afterEach(() => {
	vi.unstubAllGlobals();
	resetBindings();
});

describe("comboFromEvent", () => {
	test("names a key the way the table does", () => {
		expect(comboFromEvent(press("s"))).toBe("S");
		expect(comboFromEvent(press("S", { shiftKey: true }))).toBe("Shift+S");
		expect(comboFromEvent(press("+", { shiftKey: true }))).toBe("+");
		expect(comboFromEvent(press("z", { ctrlKey: true }))).toBe("Mod+Z");
		expect(comboFromEvent(press("ArrowLeft", { shiftKey: true }))).toBe(
			"Shift+Left",
		);
		expect(comboFromEvent(press(" "))).toBe("Space");
		expect(comboFromEvent(press("Shift", { shiftKey: true }))).toBeNull();
	});
});

describe("rebinding", () => {
	test("a free key takes the row and the old key stops matching", () => {
		expect(rebind("split", "X")).toEqual({ ok: true });
		expect(getBindings().split).toBe("X");
		expect(bindingFor(press("x"), ["split"])).toBe("split");
		expect(bindingFor(press("b", { ctrlKey: true }), ["split"])).toBeNull();
	});

	test("a key of another row is refused and names that row", () => {
		expect(rebind("split", "C")).toEqual({
			ok: false,
			conflict: "add_caption",
		});
		expect(rebind("split", "Mod+Z")).toEqual({ ok: false, conflict: "undo" });
		expect(getBindings().split).toBe("Mod+B");
	});

	test("S is the preview axis of the compatible map, Q and W are the trims", () => {
		expect(DEFAULT_BINDINGS.split).toBe("Mod+B");
		expect(DEFAULT_BINDINGS.trim_start).toBe("Q");
		expect(DEFAULT_BINDINGS.trim_end).toBe("W");
		expect(bindingFor(press("s"), ["split"])).toBeNull();
		expect(bindingFor(press("["), ["trim_start", "trim_end"])).toBeNull();
		expect(bindingFor(press("q"), ["trim_start", "trim_end"])).toBe(
			"trim_start",
		);
	});

	test("a key of a fixed row of the compatible map is refused", () => {
		expect(findConflict("add_text", "Mod+B")).toBe("split");
		expect(findConflict("add_text", "Mod+Shift+B")).toBe("split_all");
		expect(findConflict("add_text", "[")).toBe("select_side");
		expect(findConflict("add_text", "Mod+E")).toBe("export");
	});

	test("the aliases + and - are held by the zoom rows", () => {
		expect(findConflict("add_text", "+")).toBe("zoom_in");
		expect(findConflict("add_text", "-")).toBe("zoom_out");
		expect(findConflict("zoom_in", "+")).toBeNull();
	});

	test("reserved keys and Alt combos are refused", () => {
		expect(findConflict("split", "Tab")).toBe("reserved");
		expect(findConflict("split", "Alt+X")).toBe("reserved");
	});

	test("only the overrides are stored, and reset clears them", () => {
		const data = new Map<string, string>();
		vi.stubGlobal("localStorage", {
			getItem: (k: string) => data.get(k) ?? null,
			setItem: (k: string, v: string) => void data.set(k, v),
			removeItem: (k: string) => void data.delete(k),
		});
		rebind("split", "X");
		expect(data.get("vv.keymap")).toBe('{"split":"X"}');
		resetBindings();
		expect(data.has("vv.keymap")).toBe(false);
		expect(getBindings()).toEqual(DEFAULT_BINDINGS);
	});

	test("Mod+= zooms in, with + and = as aliases, and Mod+- and - zoom out", () => {
		expect(bindingFor(press("=", { ctrlKey: true }), ["zoom_in"])).toBe(
			"zoom_in",
		);
		expect(bindingFor(press("+", { shiftKey: true }), ["zoom_in"])).toBe(
			"zoom_in",
		);
		expect(bindingFor(press("="), ["zoom_in"])).toBe("zoom_in");
		expect(bindingFor(press("-", { ctrlKey: true }), ["zoom_out"])).toBe(
			"zoom_out",
		);
		expect(bindingFor(press("-"), ["zoom_out"])).toBe("zoom_out");
	});

	test("a rebound zoom row drops its aliases", () => {
		rebind("zoom_in", "X");
		expect(bindingFor(press("+", { shiftKey: true }), ["zoom_in"])).toBeNull();
		expect(bindingFor(press("x"), ["zoom_in"])).toBe("zoom_in");
	});
});
