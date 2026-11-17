import { describe, expect, it } from "vitest";
import nativeMenu from "../../../../../desktop/shell/menu.go?raw";

import { SHORTCUTS } from "./keymap";
import { MENU_ACTION_IDS, MENUS, type MenuItemDef } from "./menu";

const items: MenuItemDef[] = MENUS.flatMap((menu) =>
	menu.items.filter((entry): entry is MenuItemDef => entry !== "separator"),
);

describe("the editor menu table", () => {
	it("holds only ids the runner can run", () => {
		for (const item of items) expect(MENU_ACTION_IDS).toContain(item.id);
	});

	it("names only rows of the cheat sheet for its keys", () => {
		const rows = new Set(SHORTCUTS.map((row) => row.id));
		for (const item of items) {
			if (item.row !== undefined) expect(rows.has(item.row)).toBe(true);
		}
	});

	it("has no id twice", () => {
		const ids = items.map((item) => item.id);
		expect(new Set(ids).size).toBe(ids.length);
	});

	// The native menu bar of the desktop shell is built from desktop/shell/menu.go;
	// an action id it sends that the page does not know would do nothing.
	it("runs every action of the native menu of the desktop shell", () => {
		const native = [...nativeMenu.matchAll(/Action: "([^"]+)"/g)].map(
			(m) => m[1],
		);
		expect(native.length).toBeGreaterThan(20);
		for (const id of native) expect(MENU_ACTION_IDS).toContain(id);
		expect(new Set(native)).toEqual(new Set(items.map((item) => item.id)));
	});
});
