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

// `.key`, as it is or with its case changed.
const KEY = String.raw`\.key(?:\.to(?:Lower|Upper)Case\(\))?(?![\w(])`;
const KEY_EQ = new RegExp(`${KEY}\\s*[!=]==?\\s*"([^"]*)"`, "g");
const EQ_KEY = new RegExp(`"([^"]*)"\\s*[!=]==?\\s*[\\w.]*${KEY}`, "g");
// A list of literals asked whether it holds the key, and a regular expression
// run on it: every literal of the list counts, and a pattern always does.
const LIST_HAS_KEY = new RegExp(`\\[([^\\]]*)\\]\\.includes\\([\\w.]*${KEY}`);
const PATTERN_ON_KEY = new RegExp(`/[a-z]*\\.test\\([\\w.]*${KEY}`);
// An event has a physical `code`; an error object has one too, so only a
// variable named like an event counts.
const EVENT = "(?:event|e|ev|evt|nativeEvent)";
const EVENT_CODE = new RegExp(`(?:^|[^\\w.])${EVENT}\\.code\\s*[!=]==?\\s*"`);
// A variable the key is copied to: `const pressed = event.key`, or
// `const { key } = event` (also `{ key: pressed }`).
const KEY_COPY = new RegExp(
	`(?:const|let|var)\\s+(\\w+)\\s*=\\s*[\\w.]*${KEY}`,
);
const KEY_DESTRUCTURE = new RegExp(
	`\\{[^}]*\\bkey\\b(?:\\s*:\\s*(\\w+))?[^}]*\\}\\s*=\\s*[\\w.]*${EVENT}\\b`,
);
const KEY_SWITCH = new RegExp(
	`switch\\s*\\([^)]*(?:${KEY}|(?:^|[^\\w.])${EVENT}\\.code)[^)]*\\)`,
);
const CASE_LABEL = /^\s*case "([^"]*)":/;
// A literal that can only be a key: one character, a function key, or a key
// name that is not a widget key. A variable named `key` compared to one of
// these is a key too (a helper that takes `event.key`), while `key === "items"`
// (a key of an object) is not.
const KEY_LIKE = /^(?:.|F\d{1,2}|Space|Delete|Backspace|Insert)$/;

function escapeRegExp(text: string): string {
	return text.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// The literals `name` is compared to on a line.
function comparedTo(name: string, code: string): string[] {
	const id = escapeRegExp(name);
	const out: string[] = [];
	const left = new RegExp(`(?:^|[^\\w.])${id}\\s*[!=]==?\\s*"([^"]*)"`, "g");
	const right = new RegExp(`"([^"]*)"\\s*[!=]==?\\s*${id}(?![\\w.(])`, "g");
	for (const m of code.matchAll(left)) if (m[1] !== undefined) out.push(m[1]);
	for (const m of code.matchAll(right)) if (m[1] !== undefined) out.push(m[1]);
	return out;
}

// The lines of `source` that compare a key event against a literal that is a
// shortcut: a letter, a digit, a symbol, a function key, Space, Delete,
// Backspace, or any physical `code`, directly or through a copy of the key.
function hardCodedKeys(file: string, source: string): Finding[] {
	const lines = source.split("\n");
	const found: Finding[] = [];
	// The `case` labels that follow a `switch` on a key are keys too.
	let keySwitchUntil = -1;
	// The variables of the file the key was copied to.
	const copies = new Set<string>();
	lines.forEach((text, index) => {
		const code = text.replace(/\/\/.*$/, "");
		const copy = KEY_COPY.exec(code) ?? KEY_DESTRUCTURE.exec(code);
		if (copy !== null) copies.add(copy[1] ?? "key");
		const switchOnCopy = [...copies].some((name) =>
			new RegExp(`switch\\s*\\(\\s*${escapeRegExp(name)}\\s*\\)`).test(code),
		);
		if (KEY_SWITCH.test(code) || switchOnCopy) keySwitchUntil = index + 30;
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
		const list = LIST_HAS_KEY.exec(code)?.[1];
		if (list !== undefined) {
			for (const m of list.matchAll(/"([^"]*)"/g)) {
				if (m[1] !== undefined) literals.push(m[1]);
			}
		}
		if (PATTERN_ON_KEY.test(code)) literals.push("pattern");
		for (const name of copies) literals.push(...comparedTo(name, code));
		if (!copies.has("key")) {
			literals.push(
				...comparedTo("key", code).filter((literal) => KEY_LIKE.test(literal)),
			);
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

	test("a key copied to a variable, a list, a regular expression or a key parameter is found", () => {
		const source = [
			"const pressed = event.key.toLowerCase();",
			'if (pressed === "s") save();',
			"const { key } = event;",
			'if (key === "j") back();',
			'if (["Delete", "Backspace"].includes(event.key)) remove();',
			"if (/^[jkl]$/i.test(event.key)) shuttle();",
			'if (event.key.toUpperCase() === "K") play();',
			'const isRemove = (key: string): boolean => key === "Delete";',
			"switch (pressed) {",
			'\tcase "x":',
			"}",
		].join("\n");
		expect(hardCodedKeys("x.ts", source).map((f) => f.line)).toEqual([
			2, 4, 5, 6, 7, 8, 10,
		]);
	});

	test("a variable named key that holds no key event and no key-like literal passes", () => {
		const source = [
			'schema = key === "items" ? schema?.items : schema;',
			'if (["Enter", "Escape"].includes(event.key)) close();',
		].join("\n");
		expect(hardCodedKeys("x.ts", source)).toEqual([]);
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
