import { describe, expect, test } from "vitest";

import { panelShortcut } from "./keymap";

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
