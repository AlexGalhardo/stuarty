import { describe, expect, test } from "vitest";

import { editShortcut, panelShortcut } from "./keymap";

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
		expect(edit("d")).toBeNull();
		expect(edit("Delete", { altKey: true })).toBeNull();
	});
});
