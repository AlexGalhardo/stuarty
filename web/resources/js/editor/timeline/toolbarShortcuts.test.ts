import { describe, expect, test } from "vitest";

import { rowById } from "../commands/keymap";
import { getKeymap, keyLabel } from "../commands/keymapStore";
import toolbar from "./TimelineToolbar.tsx?raw";

// Epic 85: every shortcut the toolbar has is shown in the tooltip of its button, read from the registry,
// so a rebinding is live. A tooltip row that is not in the registry, or has no key, would show a name with
// nothing beside it.
const rows = [...toolbar.matchAll(/tip\([^;]*?,\s*"([a-z_]+)"\)/g)].map(
	(match) => match[1] ?? "",
);

describe("timeline toolbar tooltips", () => {
	test("name the select, blade, magnet and link tools by their key", () => {
		for (const row of ["tool_select", "tool_blade", "toggle_magnet", "link"]) {
			expect(rows).toContain(row);
		}
	});

	test.each([...new Set(rows)])("row %s has a key in the registry", (row) => {
		expect(rowById(row)).toBeDefined();
		expect(keyLabel(row, getKeymap(), false)).not.toBeNull();
	});
});
