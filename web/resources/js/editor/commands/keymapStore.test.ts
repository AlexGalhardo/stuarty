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
		expect(bindingFor(press("s"), ["split"])).toBeNull();
	});

	test("a key of another row is refused and names that row", () => {
		expect(rebind("split", "C")).toEqual({
			ok: false,
			conflict: "add_caption",
		});
		expect(rebind("split", "Mod+Z")).toEqual({ ok: false, conflict: "undo" });
		expect(getBindings().split).toBe("S");
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

	test("= still zooms in while + is the key", () => {
		expect(bindingFor(press("="), ["zoom_in"])).toBe("zoom_in");
	});
});
