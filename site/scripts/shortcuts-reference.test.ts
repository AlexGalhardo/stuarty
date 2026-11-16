import assert from "node:assert/strict";
import path from "node:path";
import { test } from "node:test";
import { readKeymap, shortcutRows, shortcutsPage } from "./shortcuts-reference.ts";

const root = path.join(import.meta.dirname, "..", "..");

test("every row of the editor keymap is on the page, with Mod spelled out per platform", async () => {
	const { keymap, labels } = await readKeymap(root);
	const page = shortcutsPage(keymap, labels);
	for (const row of shortcutRows(keymap, labels)) assert.ok(page.includes(row.label.replace(/\|/g, "\|")), `the page lacks "${row.label}"`);
	assert.match(page, /`Ctrl\+B`.*`Cmd\+B`/);
	assert.doesNotMatch(page.split("
").filter((l) => l.startsWith("|")).join("
"), /`Mod\+/, "Mod is always replaced by Ctrl or Cmd in a key cell");
});

test("a row without a label in en.json fails the generation", async () => {
	const { keymap } = await readKeymap(root);
	assert.throws(() => shortcutRows(keymap, {}), /no ui\.editor\.shortcuts\.rows/);
});
