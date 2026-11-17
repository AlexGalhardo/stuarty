import { describe, expect, test } from "vitest";

import actionsSource from "./actions.ts?raw";
import {
	BINDING_IDS,
	DEFAULT_BINDINGS,
	editShortcut,
	fixedRowFor,
	isSelectAllShortcut,
	labelWithKey,
	landmarkShortcut,
	panelShortcut,
	SHORTCUTS,
	UI_SHORTCUT_ACTIONS,
} from "./keymap";

const key = (init: Partial<Parameters<typeof panelShortcut>[0]>) =>
	panelShortcut({
		key: "1",
		code: "Digit1",
		ctrlKey: false,
		metaKey: false,
		altKey: true,
		shiftKey: false,
		...init,
	});

describe("panelShortcut", () => {
	test("Alt+1, Alt+2 and Alt+3 name the library, inspector and history", () => {
		expect(key({})).toBe("library");
		expect(key({ key: "2", code: "Digit2" })).toBe("inspector");
		expect(key({ key: "3", code: "Digit3" })).toBe("history");
	});

	test("the physical key counts when Option changes the character", () => {
		expect(key({ key: "\u00a1", code: "Digit1" })).toBe("library");
	});

	test("Alt is required, Ctrl, Cmd and Shift are not allowed", () => {
		expect(key({ altKey: false })).toBeNull();
		expect(key({ ctrlKey: true })).toBeNull();
		expect(key({ metaKey: true })).toBeNull();
		expect(key({ shiftKey: true })).toBeNull();
		expect(key({ key: "4", code: "Digit4" })).toBeNull();
	});
});

describe("editShortcut", () => {
	const edit = (
		name: string,
		mods: Partial<
			Record<"ctrlKey" | "metaKey" | "altKey" | "shiftKey", boolean>
		> = {},
	) =>
		editShortcut({
			key: name,
			ctrlKey: false,
			metaKey: false,
			altKey: false,
			shiftKey: false,
			...mods,
		});

	test("the keys of the delete, duplicate and rename gestures", () => {
		expect(edit("Delete")).toBe("delete");
		expect(edit("Backspace")).toBe("delete");
		expect(edit("Delete", { shiftKey: true })).toBe("ripple_delete");
		expect(edit("d", { ctrlKey: true })).toBe("duplicate");
		expect(edit("D", { metaKey: true })).toBe("duplicate");
		expect(edit("F2")).toBe("rename");
		expect(edit("v")).toBe("toggle_enabled");
		expect(edit("~", { shiftKey: true })).toBe("toggle_link");
		expect(edit("S", { ctrlKey: true, shiftKey: true })).toBe("extract_audio");
		expect(edit("s", { metaKey: true, shiftKey: true })).toBe("extract_audio");
		expect(edit("C", { ctrlKey: true, shiftKey: true })).toBe(
			"copy_attributes",
		);
		expect(edit("v", { metaKey: true, shiftKey: true })).toBe(
			"paste_attributes",
		);
		expect(edit("c", { ctrlKey: true })).toBeNull();
		expect(edit("p")).toBe("toggle_magnet");
		expect(edit("P")).toBe("toggle_magnet");
		expect(edit("p", { shiftKey: true })).toBeNull();
		expect(edit("p", { ctrlKey: true })).toBeNull();
		expect(edit("s", { ctrlKey: true })).toBeNull();
		expect(edit("V", { shiftKey: true })).toBeNull();
		expect(edit("v", { ctrlKey: true })).toBeNull();
		expect(edit("d")).toBeNull();
		expect(edit("Delete", { altKey: true })).toBeNull();
	});
});

describe("the table of SDD-012 section 18.2", () => {
	const body =
		/export interface Actions \{([\s\S]*?)\n\}/.exec(actionsSource)?.[1] ?? "";
	const methods = new Set(
		[...body.matchAll(/^\t(\w+)[(<]/gm)].map((m) => m[1] as string),
	);

	test("every shortcut resolves to an action of actions.ts or a listed editor surface", () => {
		expect(methods.size).toBeGreaterThan(20);
		for (const row of SHORTCUTS) {
			const known =
				methods.has(row.action) ||
				(UI_SHORTCUT_ACTIONS as readonly string[]).includes(row.action);
			expect(known, `${row.keys} -> ${row.action}`).toBe(true);
		}
	});

	test("the keys the section lists are all in the table", () => {
		const keys = SHORTCUTS.map((row) => row.keys).join(" | ");
		for (const key of [
			"Space",
			"S",
			"Mod+Z",
			"Ctrl+Y",
			"Mod+D",
			"C",
			"T",
			"F2",
			"V",
			"N",
			"Mod+K",
			"Alt+1",
			"Mod+A",
			"Esc",
			"F6",
		]) {
			expect(keys, key).toContain(key);
		}
	});
});

describe("the CapCut-compatible map of SDD-012 section 18.7", () => {
	// Every key a row names, with the aliases, so a key held by two rows shows.
	const held = SHORTCUTS.flatMap((row) =>
		[...row.keys.split(", "), ...(row.aliases?.split(", ") ?? [])]
			.filter((key) => !key.startsWith("Timeline"))
			.map((key) => ({ id: row.id, key })),
	);

	test("no key is held by two rows", () => {
		const seen = new Map<string, string>();
		for (const { id, key } of held) {
			expect(
				seen.get(key),
				`${key}: ${id} and ${seen.get(key)}`,
			).toBeUndefined();
			seen.set(key, id);
		}
	});

	test("the rebindable rows are rows of the table with a default key", () => {
		for (const id of BINDING_IDS) {
			const row = SHORTCUTS.find((candidate) => candidate.id === id);
			expect(row, id).toBeDefined();
			expect(row?.keys, id).not.toBe("");
		}
	});

	test("the keys the browser reserves are not bound on the web", () => {
		const reserved = ["Mod+N", "Mod+T", "Mod+W", "Mod+Shift+N", "Mod+Shift+T"];
		const reservedMore = ["Mod+Q", "Mod+Tab", "Mod+R", "Mod+Shift+F", "Mod+O"];
		for (const { key } of held) {
			expect(reserved, key).not.toContain(key);
			expect(reservedMore, key).not.toContain(key);
		}
	});

	test("old keys of the first map do not stay on a row", () => {
		const keys = held.map((entry) => entry.key);
		expect(keys).not.toContain("S");
		expect(keys).not.toContain("[ ");
		expect(fixedRowFor("Mod+Shift+B")).toBe("split_all");
		expect(fixedRowFor("[")).toBe("select_side");
		expect(fixedRowFor("]")).toBe("select_side");
		expect(fixedRowFor("Mod+C")).toBe("copy");
	});

	test("the marker, speed curve and scroll keys are rows of the table", () => {
		expect(fixedRowFor("Shift+M")).toBe("next_marker");
		expect(fixedRowFor("Alt+Shift+M")).toBe("next_marker");
		expect(fixedRowFor("Shift+B")).toBe("speed_curve");
		expect(fixedRowFor("Alt+Wheel")).toBe("scroll");
	});
});

describe("selectAll and landmark keys", () => {
	const base = {
		ctrlKey: false,
		metaKey: false,
		altKey: false,
		shiftKey: false,
	};

	test("Mod+A selects, A and Mod+Shift+A do not", () => {
		expect(isSelectAllShortcut({ ...base, key: "a", ctrlKey: true })).toBe(
			true,
		);
		expect(isSelectAllShortcut({ ...base, key: "A", metaKey: true })).toBe(
			true,
		);
		expect(isSelectAllShortcut({ ...base, key: "a" })).toBe(false);
		expect(
			isSelectAllShortcut({ ...base, key: "a", ctrlKey: true, shiftKey: true }),
		).toBe(false);
	});

	test("F6 goes forward, Shift+F6 back, with Ctrl or Alt it is the browser's", () => {
		expect(landmarkShortcut({ ...base, key: "F6" })).toBe(1);
		expect(landmarkShortcut({ ...base, key: "F6", shiftKey: true })).toBe(-1);
		expect(landmarkShortcut({ ...base, key: "F6", ctrlKey: true })).toBeNull();
		expect(landmarkShortcut({ ...base, key: "F6", altKey: true })).toBeNull();
		expect(landmarkShortcut({ ...base, key: "F5" })).toBeNull();
	});
});

describe("labelWithKey", () => {
	test("adds the first key of the row, Mod by platform", () => {
		expect(labelWithKey("Split", "split", DEFAULT_BINDINGS, false)).toBe(
			"Split (Ctrl+B)",
		);
		expect(labelWithKey("Split", "split", DEFAULT_BINDINGS, true)).toBe(
			"Split (Cmd+B)",
		);
		expect(labelWithKey("Delete", "delete", DEFAULT_BINDINGS, false)).toBe(
			"Delete (Delete)",
		);
	});

	test("follows a rebound key", () => {
		expect(
			labelWithKey(
				"Split",
				"split",
				{ ...DEFAULT_BINDINGS, split: "S" },
				false,
			),
		).toBe("Split (S)");
	});

	test("leaves the label alone for an unknown row", () => {
		expect(labelWithKey("Fit", "nope", DEFAULT_BINDINGS, false)).toBe("Fit");
	});
});
