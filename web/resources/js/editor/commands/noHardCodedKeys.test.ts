import { describe, expect, test } from "vitest";

// SDD-012 section 18.9 item 6: no handler of the editor names a key. Every
// shortcut is a row of the registry (commands/keymap.ts) read through
// keymapStore.ts, so a rebinding is live and the cheat sheet never lies. A
// widget key (the arrows of a slider, Enter, Esc, Tab, Home and End) is not a
// shortcut and may be named; a line that must keep another literal says why in
// a comment `shortcut-ok: <reason>` on the same line or the line above.

const sources = import.meta.glob("../**/*.{ts,tsx}", {
	query: "?raw",
	import: "default",
	eager: true,
}) as Record<string, string>;

const WIDGET_KEYS = new Set([
	"Enter",
	"Escape",
	"Tab",
	"ArrowLeft",
	"ArrowRight",
	"ArrowUp",
	"ArrowDown",
	"Home",
	"End",
	"PageUp",
	"PageDown",
]);

// The file that may write a key: the registry itself.
const EXEMPT = /(^|\/)commands\/keymap\.ts$/;

function isSource(path: string): boolean {
	return (
		!/\.test\.tsx?$/.test(path) &&
		!path.includes("/fixtures/") &&
		!EXEMPT.test(path)
	);
}

interface Finding {
	file: string;
	line: number;
	text: string;
}

const KEY_EQ = /\.key(?:\.toLowerCase\(\))?\s*[!=]==?\s*"([^"]*)"/g;
const EQ_KEY = /"([^"]*)"\s*[!=]==?\s*[\w.]*\.key(?:\.toLowerCase\(\))?/g;
// An event has a physical `code`; an error object has one too, so only a
// variable named like an event counts.
const EVENT_CODE =
	/(?:^|[^\w.])(?:event|e|ev|evt|nativeEvent)\.code\s*[!=]==?\s*"/;
const KEY_SWITCH =
	/switch\s*\([^)]*(?:\.key(?![\w])|(?:^|[^\w.])(?:event|e|ev|evt|nativeEvent)\.code)[^)]*\)/;
const CASE_LABEL = /^\s*case "([^"]*)":/;

// The lines of `source` that compare a key event against a literal that is a
// shortcut: a letter, a digit, a symbol, a function key, Space, Delete,
// Backspace, or any physical `code`.
function hardCodedKeys(file: string, source: string): Finding[] {
	const lines = source.split("\n");
	const found: Finding[] = [];
	// The `case` labels that follow a `switch` on a key are keys too.
	let keySwitchUntil = -1;
	lines.forEach((text, index) => {
		const code = text.replace(/\/\/.*$/, "");
		if (KEY_SWITCH.test(code)) keySwitchUntil = index + 30;
		const ok =
			text.includes("shortcut-ok:") ||
			(lines[index - 1] ?? "").includes("shortcut-ok:");
		if (ok) return;
		const literals: string[] = [];
		for (const m of code.matchAll(KEY_EQ)) {
			if (m[1] !== undefined) literals.push(m[1]);
		}
		for (const m of code.matchAll(EQ_KEY)) {
			if (m[1] !== undefined) literals.push(m[1]);
		}
		if (EVENT_CODE.test(code)) literals.push("code");
		if (index <= keySwitchUntil) {
			const label = CASE_LABEL.exec(code);
			if (label?.[1] !== undefined) literals.push(label[1]);
		}
		if (literals.some((literal) => !WIDGET_KEYS.has(literal))) {
			found.push({ file, line: index + 1, text: text.trim() });
		}
	});
	return found;
}

describe("no handler of the editor hard-codes a key", () => {
	test("the files of the editor name no shortcut key outside the registry", () => {
		const found = Object.entries(sources)
			.filter(([path]) => isSource(path))
			.flatMap(([path, source]) => hardCodedKeys(path, source));
		expect(
			found.map((f) => `${f.file}:${f.line}  ${f.text}`),
			"move the key to a row of commands/keymap.ts, or say why on a `shortcut-ok:` comment",
		).toEqual([]);
	});

	test("it sees the files of the editor", () => {
		expect(
			Object.keys(sources).filter((path) => isSource(path)).length,
		).toBeGreaterThan(100);
	});
});

describe("the scan fails on a hard-coded key", () => {
	test("a letter, a digit, a function key and a physical code are found", () => {
		const source = [
			'if (event.key === "s") save();',
			'if (event.key.toLowerCase() === "k") play();',
			'if (event.key === "F2") rename();',
			'if (event.code === "KeyX") clear();',
			'if ("1" === event.key) panel();',
			"switch (event.key) {",
			'\tcase "Delete":',
			"}",
		].join("\n");
		expect(hardCodedKeys("x.ts", source).map((f) => f.line)).toEqual([
			1, 2, 3, 4, 5, 7,
		]);
	});

	test("widget keys, an error code and a justified line pass", () => {
		const source = [
			'if (event.key === "Enter") commit();',
			'if (event.key === "Escape") close();',
			'if (event.key === "ArrowLeft") back();',
			'if (error.code === "E_FORBIDDEN") deny();',
			"// shortcut-ok: the point of a curve is deleted with Delete",
			'if (event.key === "Delete") remove();',
			'if (event.key === "Backspace") remove(); // shortcut-ok: same',
			"if (event.key === name) other();",
		].join("\n");
		expect(hardCodedKeys("x.ts", source)).toEqual([]);
	});
});
