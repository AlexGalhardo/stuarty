import { describe, expect, test } from "vitest";

import keys from "../keys.ts?raw";
import operations from "./operations.tsx?raw";
import { SHORTCUTS } from "./shortcuts";
import toolbar from "./Toolbar.tsx?raw";

// The keymap of SDD-012 section 24.10, as the toolbar names it in its tooltips.
describe("toolbar shortcuts", () => {
	test("no button writes its key as a literal: it names a row of the table", () => {
		for (const source of [operations, toolbar]) {
			expect(source).not.toMatch(/shortcut(=|: )"[^"]*[^a-zA-Z"][^"]*"/);
		}
	});

	test("every row a button names exists in the table", () => {
		const rows = [operations, toolbar].flatMap((source) =>
			[...source.matchAll(/shortcut(?:=|: )"([a-zA-Z]+)"/g)].map(
				(m) => m[1] ?? "",
			),
		);
		expect(rows.length).toBeGreaterThan(8);
		for (const row of rows) expect(SHORTCUTS).toHaveProperty(row);
	});

	test("each key of the table is one the editor handles", () => {
		const handled: Record<string, RegExp> = {
			undo: /key === "z"/,
			redo: /key === "y"/,
			group: /key === "g"/,
			ungroup: /key === "g"/,
			duplicate: /key === "d"/,
			remove: /Delete/,
			forward: /"\]"/,
			back: /"\["/,
			zoomIn: /"\+"/,
			zoomOut: /"-"/,
			fit: /"0"/,
			actual: /"1"/,
		};
		for (const row of Object.keys(SHORTCUTS)) {
			expect(handled[row], row).toBeDefined();
			expect(keys, row).toMatch(handled[row] as RegExp);
		}
	});
});
