// The internationalization check of SDD-012 section 16.4.
//
//   bun run check:i18n     the tree, then the fixtures
//
// I1, I2   en.json and pt-BR.json have the same keys and the same placeholders
// I3       no literal user-facing string in any source of resources/js (the editor,
//          components, layouts and pages; generated code and tests excepted): JSX text with a letter, and
//          the attributes aria-label, aria-description, title, placeholder, alt
//          and label with a literal value, must go through t()
// I4       every key used in code exists in en.json, and every ui.editor key of
//          en.json is used (a template such as `ui.editor.errors.${code}` uses
//          the keys under its prefix)
// I5       every code of the generated error table has ui.editor.errors.<CODE>,
//          every non-internal command has ui.editor.commands.<name>.title and
//          every lint rule has ui.editor.lint.<code>
// I6       all of I1 to I5 and I7 run, and every rule has a failing and a passing
//          fixture under i18n-fixtures (checked below)
// I7       (proposed) no key has the English text in pt-BR, outside SAME_IN_PORTUGUESE
//
// The syntax tree comes from the API inside the `typescript` package, as in
// check-thin-client.ts. No new dependency.

import { existsSync, readdirSync, readFileSync } from "node:fs";
import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { SyntaxKind } from "typescript/unstable/ast";
import { API } from "typescript/unstable/sync";

import {
	type Catalog,
	catalogFixtureProblems,
	compareCatalogs,
	duplicateKeys,
	flatten,
	type Leaf,
	missingKeys,
	RULES,
	untranslated,
} from "./i18n-catalog.ts";

type Node = import("typescript/unstable/ast").Node;
type SourceFile = import("typescript/unstable/ast").SourceFile;

interface Finding {
	file: string;
	line: number;
	rule: string;
	message: string;
}

interface Usage {
	// Keys written out in code, with where.
	keys: { key: string; file: string; line: number }[];
	// Prefixes of template keys: `ui.editor.errors.${code}` gives "ui.editor.errors.".
	prefixes: { prefix: string; file: string; line: number }[];
}

const LITERAL_ATTRIBUTES = new Set([
	"aria-label",
	"aria-description",
	"title",
	"placeholder",
	"alt",
	"label",
]);
const KEY = /^ui(\.[A-Za-z0-9_]+)+$/;
const LETTER = /\p{L}/u;
// Literals that are not words of a language (rule T9): example values that show a
// format. Add here with the reason when one is needed; nothing else is allowed.
const TECHNICAL_LITERALS: ReadonlySet<string> = new Set([
	"sha256:…", // a content hash, the placeholder of the brand kit logo field
	"prj_...", // a project id, the placeholder of the API key project field
]);
const isWord = (text: string): boolean => LETTER.test(text) && !TECHNICAL_LITERALS.has(text);

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = join(here, "..");

// Sources whose literals are not user-facing: tests, generated code and the
// editor's own test fixtures.
function isExempt(file: string): boolean {
	return (
		/\.test\.tsx?$/.test(file) || file.includes("/generated/") || file.includes("/editor/fixtures/")
	);
}

function loadCatalog(name: string): Catalog {
	return JSON.parse(readFileSync(join(webRoot, "resources/lang", name), "utf8")) as Catalog;
}

function catalogDuplicates(name: string): Finding[] {
	const text = readFileSync(join(webRoot, "resources/lang", name), "utf8");
	return duplicateKeys(text, `resources/lang/${name}`).map((p) => ({
		file: `resources/lang/${name}`,
		line: 0,
		rule: p.rule,
		message: p.message,
	}));
}

function literalText(n: Node): string | null {
	if (
		n.kind === SyntaxKind.StringLiteral ||
		n.kind === SyntaxKind.NoSubstitutionTemplateLiteral
	) {
		return n.text;
	}
	return null;
}

function isCallTo(n: Node, name: string): boolean {
	return n.kind === SyntaxKind.CallExpression && n.expression.kind === SyntaxKind.Identifier && n.expression.text === name;
}

function scanFile(
	sf: SourceFile,
	fileLabel: string,
	catalog: Map<string, Leaf>,
	usage: Usage,
): Finding[] {
	const findings: Finding[] = [];
	const lineOf = (n: Node): number =>
		sf.getLineAndCharacterOfPosition(n.getStart(sf)).line + 1;
	const report = (n: Node, rule: string, message: string): void => {
		findings.push({ file: fileLabel, line: lineOf(n), rule, message });
	};

	const noteKey = (n: Node, key: string): void => {
		usage.keys.push({ key, file: fileLabel, line: lineOf(n) });
	};

	const visit = (n: Node): void => {
		switch (n.kind) {
			case SyntaxKind.JsxText: {
				if (!n.containsOnlyTriviaWhiteSpaces && isWord(n.text)) {
					report(n, "I3", `literal text "${n.text.trim()}" must go through t()`);
				}
				break;
			}
			case SyntaxKind.JsxExpression: {
				const text = n.expression ? literalText(n.expression) : null;
				const parentIsAttribute = n.parent?.kind === SyntaxKind.JsxAttribute;
				if (!parentIsAttribute && text !== null && isWord(text)) {
					report(n, "I3", `literal text "${text}" must go through t()`);
				}
				break;
			}
			case SyntaxKind.JsxAttribute: {
				if (n.name.kind !== SyntaxKind.Identifier) break;
				const name = n.name.text;
				if (!LITERAL_ATTRIBUTES.has(name) || n.initializer === undefined) break;
				const init = n.initializer;
				const text =
					literalText(init) ??
					(init.kind === SyntaxKind.JsxExpression && init.expression
						? literalText(init.expression)
						: null);
				if (text !== null && isWord(text)) {
					report(n, "I3", `${name}="${text}" must go through t()`);
				}
				break;
			}
			case SyntaxKind.StringLiteral:
			case SyntaxKind.NoSubstitutionTemplateLiteral: {
				if (KEY.test(n.text)) noteKey(n, n.text);
				break;
			}
			case SyntaxKind.TemplateExpression: {
				if (n.head.text.startsWith("ui.") && n.head.text.endsWith(".")) {
					usage.prefixes.push({ prefix: n.head.text, file: fileLabel, line: lineOf(n) });
				}
				break;
			}
			case SyntaxKind.CallExpression: {
				if (isCallTo(n, "t")) {
					const first = n.arguments[0];
					if (first !== undefined && first.kind === SyntaxKind.TemplateExpression) {
						// handled with the prefix above; the keys under it are checked below
					} else if (first !== undefined) {
						const text = literalText(first);
						if (text !== null && !KEY.test(text)) {
							report(first, "I4", `"${text}" is not a catalogue key`);
						}
					}
				}
				break;
			}
		}
		n.forEachChild(visit);
	};
	visit(sf);
	void catalog;
	return findings;
}

function scan(
	root: string,
	configFile: string,
	catalog: Map<string, Leaf>,
	usage: Usage,
	only: (file: string) => boolean,
): Finding[] {
	const cwd = root.split("\\").join("/");
	const api = new API({ cwd });
	try {
		const snapshot = api.updateSnapshot({ openProject: configFile.split("\\").join("/") });
		const project = snapshot.getProjects()[0];
		const findings: Finding[] = [];
		for (const file of project.program.getSourceFileNames()) {
			if (file.includes("/node_modules/") || !only(file)) continue;
			const sf = project.program.getSourceFile(file);
			if (sf === undefined) continue;
			if (isExempt(file)) continue;
			findings.push(...scanFile(sf, relative(cwd, file).split("\\").join("/"), catalog, usage));
		}
		return findings;
	} finally {
		api.close();
	}
}

function keyExists(catalog: Map<string, Leaf>, key: string): boolean {
	return catalog.has(key);
}

function prefixExists(catalog: Map<string, Leaf>, prefix: string): boolean {
	for (const key of catalog.keys()) if (key.startsWith(prefix)) return true;
	return false;
}

// I4 for the keys and prefixes seen in code.
function checkUsage(catalog: Map<string, Leaf>, usage: Usage): Finding[] {
	const findings: Finding[] = [];
	for (const { key, file, line } of usage.keys) {
		if (!keyExists(catalog, key) && !prefixExists(catalog, `${key}.`)) {
			findings.push({ file, line, rule: "I4", message: `key ${key} is not in en.json` });
		}
	}
	for (const { prefix, file, line } of usage.prefixes) {
		if (!prefixExists(catalog, prefix)) {
			findings.push({ file, line, rule: "I4", message: `no key of en.json starts with ${prefix}` });
		}
	}
	return findings;
}

const REGISTRY_PREFIXES = ["ui.editor.commands.", "ui.editor.lint."];

function unusedKeys(catalog: Map<string, Leaf>, usage: Usage): Finding[] {
	const used = new Set(usage.keys.map((k) => k.key));
	const prefixes = usage.prefixes.map((p) => p.prefix);
	const findings: Finding[] = [];
	for (const key of catalog.keys()) {
		if (!key.startsWith("ui.editor.")) continue;
		if (used.has(key) || prefixes.some((p) => key.startsWith(p))) continue;
		// Titles of the registry are read by name at run time (the palette and the
		// lint list); rule I5 checks that none is missing.
		if (REGISTRY_PREFIXES.some((p) => key.startsWith(p))) continue;
		findings.push({ file: "resources/lang/en.json", line: 0, rule: "I4", message: `key ${key} is never used` });
	}
	return findings;
}

// I5: the headline of every error code, a title for every non-internal command
// and every lint rule.
function registryKeys(): string[] {
	const source = readFileSync(join(webRoot, "resources/js/generated/errors.ts"), "utf8");
	const table = source.slice(source.indexOf("ERROR_TABLE"));
	const codes = [...table.matchAll(/^\s+(E_[A-Z_]+):/gm)].map((m) => m[1] as string);
	const extra = ["E_NETWORK", "E_UNAUTHORIZED", "E_FORBIDDEN", "unknown"];
	const commands = JSON.parse(readFileSync(join(webRoot, "resources/js/generated/commands.json"), "utf8")) as {
		name: string;
		internal: boolean;
	}[];
	const rules = JSON.parse(readFileSync(join(webRoot, "../core/schema/lint-rules.json"), "utf8")) as {
		code: string;
	}[];
	return [
		...[...codes, ...extra].map((code) => `ui.editor.errors.${code}`),
		...commands.filter((c) => !c.internal).map((c) => `ui.editor.commands.${c.name}.title`),
		...rules.map((r) => `ui.editor.lint.${r.code}`),
	];
}

function registryTitles(catalog: Map<string, Leaf>): Finding[] {
	return missingKeys(catalog, registryKeys()).map((p) => ({
		file: "resources/lang/en.json",
		line: 0,
		rule: p.rule,
		message: p.message,
	}));
}

const format = (f: Finding): string => `${f.file}:${f.line}  ${f.rule}  ${f.message}`;

// A fixture directory is <rule>-fail or <rule>-pass under i18n-fixtures: a fail
// one must report its rule, a pass one nothing.
function checkFixtures(catalog: Map<string, Leaf>): string[] {
	const dir = join(here, "i18n-fixtures");
	const usage: Usage = { keys: [], prefixes: [] };
	const findings = scan(dir, join(dir, "tsconfig.json"), catalog, usage, () => true);
	findings.push(...checkUsage(catalog, usage));
	const problems: string[] = [];
	const cases = readdirSync(dir)
		.filter((n) => /-(fail|pass)$/.test(n))
		.sort();
	for (const rule of RULES) {
		for (const kind of ["fail", "pass"]) {
			if (!cases.includes(`${rule.toLowerCase()}-${kind}`)) {
				problems.push(`rule ${rule} has no ${kind} fixture`);
			}
		}
	}
	for (const name of cases) {
		const [rule, kind] = [name.split("-")[0] as string, name.split("-").pop() as string];
		if (existsSync(join(dir, name, "en.json"))) {
			const own = catalogFixtureProblems(join(dir, name), SAME_IN_PORTUGUESE_FIXTURE).map((p) => p.rule);
			if (kind === "fail" && !own.includes(rule.toUpperCase())) {
				problems.push(`fixture ${name} should fail ${rule.toUpperCase()} and did not`);
			}
			if (kind === "pass" && own.length > 0) {
				problems.push(`fixture ${name} should pass but reports ${own.join(", ")}`);
			}
			continue;
		}
		const mine = findings.filter((f) => f.file.split("/")[0] === name);
		if (kind === "fail" && !mine.some((f) => f.rule === rule.toUpperCase())) {
			problems.push(`fixture ${name} should fail ${rule.toUpperCase()} and did not`);
		}
		if (kind === "pass" && mine.length > 0) {
			problems.push(`fixture ${name} should pass but reports: ${mine.map(format).join("; ")}`);
		}
		// I3 covers all of resources/js, not only editor/: a word outside editor/ must be reported.
		if (name === "i3-fail" && !mine.some((f) => f.rule === "I3" && !f.file.startsWith("i3-fail/editor/"))) {
			problems.push("fixture i3-fail should report a literal outside editor/ and did not");
		}
	}
	return problems;
}

// The allowlist the catalogue fixtures run with.
const SAME_IN_PORTUGUESE_FIXTURE: ReadonlySet<string> = new Set(["brand"]);

// Names, codes and formats that read the same in both languages.
const SAME_IN_PORTUGUESE: ReadonlySet<string> = new Set([
	"ui.nav.languages.en",
	"ui.nav.languages.pt-BR",
	"ui.projects.summary",
	"ui.projects.presets.9:16",
	"ui.editor.revision",
	"ui.editor.activity.revision",
	"ui.editor.timeline.item_summary",
	"ui.editor.inspector.transform.x",
	"ui.editor.inspector.transform.y",
	"ui.editor.keyframes.prop.x",
	"ui.editor.keyframes.prop.y",
	"ui.editor.keyframes.prop.volume",
	"ui.editor.clip.volume",
	"ui.editor.clip.hsl_range_name.magenta",
	"ui.editor.keyframes.easing_name.linear",
]);

const en = flatten(loadCatalog("en.json"));
const pt = flatten(loadCatalog("pt-BR.json"));

const usage: Usage = { keys: [], prefixes: [] };
const tree = scan(webRoot, join(webRoot, "tsconfig.json"), en, usage, (file) =>
	file.includes("/resources/js/"),
);
tree.push(...catalogDuplicates("en.json"), ...catalogDuplicates("pt-BR.json"));
tree.push(...checkUsage(en, usage), ...unusedKeys(en, usage), ...registryTitles(en));
for (const p of [
	...compareCatalogs(en, pt, "pt-BR"),
	...untranslated(en, pt, "pt-BR", SAME_IN_PORTUGUESE),
]) {
	tree.push({ file: "resources/lang", line: 0, rule: p.rule, message: p.message });
}
for (const f of tree) console.error(format(f));

const fixtureProblems = checkFixtures(en);
for (const p of fixtureProblems) console.error(p);

if (tree.length > 0 || fixtureProblems.length > 0) {
	console.error(`check-i18n: ${tree.length} finding(s), ${fixtureProblems.length} fixture problem(s)`);
	process.exit(1);
}
console.log("check-i18n: the catalogues agree, no literal strings, every key is used");
