import { describe, expect, test } from "vitest";

import actionsSource from "./actions.ts?raw";
import {
	BINDING_IDS,
	comboFromEvent,
	PRESET_IDS,
	type PresetId,
	presetKeys,
	RESERVED_KEYS,
	type Scope,
	SHORTCUTS,
	scopesOverlap,
	UI_SHORTCUT_ACTIONS,
} from "./keymap";

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

describe("comboFromEvent", () => {
	test("names a key the way the table does", () => {
		expect(comboFromEvent(press("s"))).toBe("S");
		expect(comboFromEvent(press("S", { shiftKey: true }))).toBe("Shift+S");
		expect(comboFromEvent(press("+", { shiftKey: true }))).toBe("+");
		expect(comboFromEvent(press("?", { shiftKey: true }))).toBe("?");
		expect(comboFromEvent(press("z", { ctrlKey: true }))).toBe("Mod+Z");
		expect(comboFromEvent(press("Z", { metaKey: true, shiftKey: true }))).toBe(
			"Mod+Shift+Z",
		);
		expect(comboFromEvent(press("ArrowLeft", { shiftKey: true }))).toBe(
			"Shift+Left",
		);
		expect(comboFromEvent(press(" "))).toBe("Space");
		expect(comboFromEvent(press("Escape"))).toBe("Esc");
		expect(comboFromEvent(press("Shift", { shiftKey: true }))).toBeNull();
	});

	test("with Alt the physical key names it, as Option changes the character", () => {
		expect(comboFromEvent(press(",", { altKey: true, code: "Comma" }))).toBe(
			"Alt+,",
		);
		// Option+. is a dot-above character on a Mac.
		expect(comboFromEvent(press("˙", { altKey: true, code: "Period" }))).toBe(
			"Alt+.",
		);
		expect(comboFromEvent(press("¡", { altKey: true, code: "Digit1" }))).toBe(
			"Alt+1",
		);
		expect(
			comboFromEvent(
				press("≠", { altKey: true, shiftKey: true, code: "Equal" }),
			),
		).toBe("Alt+Shift+=");
		expect(
			comboFromEvent(
				press("¸", { altKey: true, shiftKey: true, code: "KeyZ" }),
			),
		).toBe("Alt+Shift+Z");
		expect(
			comboFromEvent(
				press("ArrowLeft", {
					ctrlKey: true,
					altKey: true,
					shiftKey: true,
					code: "ArrowLeft",
				}),
			),
		).toBe("Mod+Alt+Shift+Left");
	});
});

describe("the table of SDD-012 sections 18.2 and 18.9", () => {
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
			expect(known, `${row.id} ${row.keys} -> ${row.action}`).toBe(true);
		}
	});

	test("row ids are unique and every rebindable row is a row of the table", () => {
		const ids = SHORTCUTS.map((row) => row.id);
		expect(new Set(ids).size).toBe(ids.length);
		for (const id of BINDING_IDS) expect(ids).toContain(id);
		expect(BINDING_IDS.length).toBe(
			SHORTCUTS.filter((row) => row.fixed !== true).length,
		);
	});

	test("every row that is not fixed has keys in both presets", () => {
		for (const row of SHORTCUTS) {
			if (row.fixed === true) continue;
			for (const preset of PRESET_IDS) {
				const keys = presetKeys(row, preset);
				expect(keys.length, `${row.id} in ${preset}`).toBeGreaterThan(0);
				for (const key of keys) expect(key, row.id).not.toBe("");
			}
		}
	});

	// Every key a row answers to in a preset, aliases included.
	const held = (preset: PresetId) =>
		SHORTCUTS.filter((row) => row.fixed !== true).flatMap((row) =>
			[...presetKeys(row, preset), ...(row.aliases?.split(", ") ?? [])].map(
				(key) => ({ id: row.id, scope: row.scope, key }),
			),
		);

	test.each(PRESET_IDS)(
		"no key is held by two rows of overlapping scopes in the %s preset",
		(preset) => {
			const seen: { id: string; scope: Scope; key: string }[] = [];
			for (const entry of held(preset)) {
				for (const other of seen) {
					if (other.key !== entry.key || other.id === entry.id) continue;
					expect(
						scopesOverlap(entry.scope, other.scope),
						`${entry.key}: ${entry.id} and ${other.id} in ${preset}`,
					).toBe(false);
				}
				seen.push(entry);
			}
		},
	);

	test.each(PRESET_IDS)(
		"no default key of the %s preset is reserved",
		(preset) => {
			for (const { id, key } of held(preset)) {
				expect(RESERVED_KEYS, `${id}: ${key}`).not.toContain(key);
			}
		},
	);

	test("the classic preset differs from the short one only where the table says", () => {
		const different = SHORTCUTS.filter((row) => row.classic !== undefined).map(
			(row) => row.id,
		);
		expect(different.sort()).toEqual(
			[
				"add_caption",
				"add_text",
				"split",
				"split_all",
				"tool_blade",
				"tool_select",
				"toggle_enabled",
				"zoom_fit",
			].sort(),
		);
	});

	test("the keys the sections list are all in the table", () => {
		const keys = held("short")
			.map((entry) => entry.key)
			.join(" | ");
		for (const key of [
			"Space",
			"Mod+Z",
			"Mod+Y",
			"Mod+D",
			"C",
			"T",
			"F2",
			"V",
			"N",
			"Mod+K",
			"Alt+1",
			"Mod+A",
			"F6",
			"?",
			"Alt+,",
			"Alt+.",
		]) {
			expect(keys, key).toContain(key);
		}
	});
});
