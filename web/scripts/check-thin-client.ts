// The thin-client check of SDD-012 section 1.4: rules R1 to R8 over every file
// under resources/js/editor/. It fails with file, line and rule id.
//
//   bun run check:thin-client          the tree, then the fixtures
//
// The syntax tree comes from the API that ships inside the `typescript`
// package (`typescript/unstable/sync`, the native compiler of TypeScript 7,
// which has no JavaScript compiler API any more). It is marked unstable, so
// `typescript` stays pinned to an exact version. No new dependency.
//
// Rule R3 is syntactic: a write on an expression rooted at a name that is
// declared with one of the document types. The strict `DeepReadonly` types of
// the read model reject the same writes at typecheck; this catches them where a
// cast or an `any` would hide them.

import { dirname, join, posix, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { SyntaxKind } from "typescript/unstable/ast";
import { API } from "typescript/unstable/sync";

type Node = import("typescript/unstable/ast").Node;
type SourceFile = import("typescript/unstable/ast").SourceFile;

interface Finding {
	file: string;
	line: number;
	rule: string;
	message: string;
}

const BANNED_PACKAGES = new Set([
	"immer",
	"mutative",
	"fast-json-patch",
	"rfc6902",
	"jsondiffpatch",
	"json-logic-js",
	"mathjs",
	"bezier-easing",
	"d3-ease",
	"d3-interpolate",
	"gsap",
	"popmotion",
]);
const GENERATED_VALUE_IMPORTS = new Set([
	"@/generated/limits",
	"@/generated/commands.json",
	"@/generated/errors",
]);
const R2_PREFIXES = [
	"interpolate",
	"ease",
	"lerp",
	"layoutText",
	"resolveKeyframe",
	"evaluateItem",
	"sourceFrame",
	"computeOverlap",
	"findFreeSlot",
	"rippleDelete",
	"applyCommand",
	"reduceTimeline",
];
const DOCUMENT_TYPES = /\b(ProjectDocument|Item|Track|Asset|Word|Keyframes)\b/;
const MUTATORS = new Set(["push", "splice", "sort", "reverse", "pop", "shift", "unshift"]);
const TIME_PROPS = new Set([
	"start",
	"duration",
	"source_in",
	"speed",
	"fps",
	"fade_in",
	"fade_out",
	"words",
]);
const R5_ALLOWLIST = [
	"timeline/geometry.ts",
	"timeline/gestures/",
	"timeline/snapping.ts",
	"timeline/items/stripTiles.ts",
	"timeline/items/waveform.ts",
	"commands/builders.ts",
	"commands/units.ts",
	"preview/",
	"ui/TimeField.tsx",
];
const STORAGE_NAMES = new Set(["localStorage", "sessionStorage", "indexedDB", "caches"]);
const PREFS_KEYS = new Set(["theme", "vv.layout", "vv.timeDisplay", "vv.timeUnit", "vv.zoomByProject", "vv.keymap", "vv.shortcutsSeen", "vv.lang"]);
const WRITE_FUNCTIONS = ["replaceDocument", "applyRevisionDiff"];

const ASSIGNMENTS = new Set<SyntaxKind>([
	SyntaxKind.EqualsToken,
	SyntaxKind.PlusEqualsToken,
	SyntaxKind.MinusEqualsToken,
	SyntaxKind.AsteriskEqualsToken,
	SyntaxKind.SlashEqualsToken,
	SyntaxKind.PercentEqualsToken,
	SyntaxKind.QuestionQuestionEqualsToken,
	SyntaxKind.BarBarEqualsToken,
	SyntaxKind.AmpersandAmpersandEqualsToken,
]);
const ARITHMETIC = new Set<SyntaxKind>([
	SyntaxKind.PlusToken,
	SyntaxKind.MinusToken,
	SyntaxKind.AsteriskToken,
	SyntaxKind.SlashToken,
	SyntaxKind.PercentToken,
]);
const COMPARISON = new Set<SyntaxKind>([
	SyntaxKind.LessThanToken,
	SyntaxKind.GreaterThanToken,
	SyntaxKind.LessThanEqualsToken,
	SyntaxKind.GreaterThanEqualsToken,
]);

// The folders of SDD-012 section 3.3 and what each may import.
const FEATURES = ["timeline", "inspector", "library", "export", "history"];
const MAY_IMPORT: Record<string, string[]> = {
	api: [],
	state: ["api"],
	commands: ["state", "api", "ui", "a11y"],
	preview: ["state", "api", "ui"],
	ui: [],
	a11y: [],
	layout: [...FEATURES, "commands", "preview", "state", "ui", "a11y"],
	boot: ["*"],
	...Object.fromEntries(
		FEATURES.map((f) => [f, ["state", "commands", "ui", "a11y", "preview", "api"]]),
	),
};

// What lies after the last /editor/ segment: "state/sync.ts".
function editorRelative(file: string): string | null {
	const marker = "/editor/";
	const at = file.lastIndexOf(marker);
	return at < 0 ? null : file.slice(at + marker.length);
}

function isExempt(rel: string): boolean {
	return rel.startsWith("fixtures/") || /\.test\.tsx?$/.test(rel);
}

const text = (n: Node | undefined): string => (n === undefined ? "" : n.getText());

function identifierName(n: Node): string | null {
	return n.kind === SyntaxKind.Identifier ? text(n) : null;
}

// The leftmost identifier of a property or element access chain.
function rootName(n: Node): string | null {
	let current: Node = n;
	for (;;) {
		if (current.kind === SyntaxKind.Identifier) return text(current);
		if (
			current.kind === SyntaxKind.PropertyAccessExpression ||
			current.kind === SyntaxKind.ElementAccessExpression ||
			current.kind === SyntaxKind.ParenthesizedExpression ||
			current.kind === SyntaxKind.NonNullExpression
		) {
			current = current.expression;
		} else return null;
	}
}

function accessedName(n: Node): string | null {
	let current: Node = n;
	while (current.kind === SyntaxKind.ParenthesizedExpression) current = current.expression;
	if (current.kind === SyntaxKind.PropertyAccessExpression) return text(current.name);
	if (current.kind === SyntaxKind.ElementAccessExpression) {
		const arg = current.argumentExpression;
		if (arg?.kind === SyntaxKind.StringLiteral) return arg.text;
	}
	return null;
}

function checkFile(sf: SourceFile, rel: string, fileLabel: string): Finding[] {
	const findings: Finding[] = [];
	const typed = new Set<string>();
	const report = (node: Node, rule: string, message: string): void => {
		findings.push({
			file: fileLabel,
			line: sf.getLineAndCharacterOfPosition(node.getStart(sf)).line + 1,
			rule,
			message,
		});
	};

	const folder = rel.split("/")[0];
	// The decode worker of the preview reads the proxy and its chunk index in byte
	// ranges (SDD-013 section 5.3); it edits nothing and holds no project data.
	// The render worker fetches the renderer module, a same-origin asset, and nothing
	// else (SDD-013 section 4.7 rule 4); the one that creates it is createRenderWorker.ts.
	const isDecodeWorker =
		rel === "preview/decode/decodeWorker.ts" || rel === "preview/render/renderWorker.ts";
	const mayCreateWorker =
		rel === "preview/decode/createWorker.ts" || rel === "preview/render/createRenderWorker.ts";
	const inApiOrBoot = folder === "api" || folder === "boot" || isDecodeWorker;
	const isSync = rel === "state/sync.ts";
	const isPrefs = rel === "state/prefs.ts";
	const r5Allowed = R5_ALLOWLIST.some((entry) => rel === entry || rel.startsWith(entry));

	// Names declared with a document type, for R3.
	const collect = (n: Node): void => {
		if (
			(n.kind === SyntaxKind.Parameter || n.kind === SyntaxKind.VariableDeclaration) &&
			n.type !== undefined &&
			DOCUMENT_TYPES.test(text(n.type))
		) {
			const name = identifierName(n.name);
			if (name) typed.add(name);
		}
		n.forEachChild(collect);
	};
	collect(sf);

	const isDocumentRoot = (n: Node): boolean => {
		const root = rootName(n);
		return root !== null && (typed.has(root) || root === "documentStore");
	};

	const visit = (n: Node): void => {
		switch (n.kind) {
			case SyntaxKind.ImportDeclaration: {
				const spec = n.moduleSpecifier.text;
				const typeOnly = n.importClause?.isTypeOnly === true;
				if (BANNED_PACKAGES.has(spec) || spec.endsWith(".wasm")) {
					report(n, "R1", `import of "${spec}" computes timelines or interpolates`);
				}
				if (
					spec.startsWith("@/generated/") &&
					!typeOnly &&
					!GENERATED_VALUE_IMPORTS.has(spec)
				) {
					report(n, "R1", `"${spec}" must be imported with import type`);
				}
				const names = text(n.importClause?.namedBindings);
				if (!isSync && WRITE_FUNCTIONS.some((fn) => new RegExp(`\\b${fn}\\b`).test(names))) {
					report(n, "R4", "the document write functions may only be imported by state/sync.ts");
				}
				checkImportGraph(n, spec);
				break;
			}
			case SyntaxKind.NewExpression: {
				const callee = text(n.expression);
				if (callee === "SharedWorker" || (callee === "Worker" && !mayCreateWorker)) {
					report(n, "R1", `new ${callee}(...) is not allowed`);
				}
				if (!inApiOrBoot && ["EventSource", "XMLHttpRequest"].includes(callee)) {
					report(n, "R6", `new ${callee} outside editor/api and editor/boot`);
				}
				break;
			}
			case SyntaxKind.FunctionDeclaration:
			case SyntaxKind.MethodDeclaration:
			case SyntaxKind.VariableDeclaration: {
				const name = n.name === undefined ? null : identifierName(n.name);
				if (name && R2_PREFIXES.some((p) => name.startsWith(p))) {
					report(n, "R2", `declaration "${name}" computes an edit result`);
				}
				break;
			}
			case SyntaxKind.CallExpression: {
				const callee = n.expression;
				const calleeText = text(callee);
				const calleeName =
					callee.kind === SyntaxKind.PropertyAccessExpression ? text(callee.name) : calleeText;
				if (R2_PREFIXES.some((p) => calleeName.startsWith(p))) {
					report(n, "R2", `call to "${calleeName}" computes an edit result`);
				}
				if (calleeName === "measureText") report(n, "R2", ".measureText() lays out text");
				if (!inApiOrBoot && (calleeText === "fetch" || calleeText === "navigator.sendBeacon")) {
					report(n, "R6", `${calleeText}() outside editor/api and editor/boot`);
				}
				if (!isSync && callee.kind === SyntaxKind.PropertyAccessExpression) {
					if (MUTATORS.has(calleeName) && isDocumentRoot(callee.expression)) {
						report(n, "R3", `.${calleeName}() mutates the read model`);
					}
					if (calleeText === "Object.assign" && n.arguments[0] && isDocumentRoot(n.arguments[0])) {
						report(n, "R3", "Object.assign mutates the read model");
					}
				}
				if (isPrefs && ["getItem", "setItem", "removeItem"].includes(calleeName)) {
					const key = n.arguments[0];
					if (key?.kind !== SyntaxKind.StringLiteral || !PREFS_KEYS.has(key.text)) {
						report(n, "R7", `storage key outside the allowed list: ${text(key)}`);
					}
				}
				break;
			}
			case SyntaxKind.DeleteExpression: {
				if (!isSync && isDocumentRoot(n.expression)) report(n, "R3", "delete on the read model");
				break;
			}
			case SyntaxKind.BinaryExpression: {
				const op = n.operatorToken.kind;
				if (!isSync && ASSIGNMENTS.has(op) && isDocumentRoot(n.left)) {
					const plain = n.left.kind === SyntaxKind.Identifier;
					if (!plain) report(n, "R3", "assignment on the read model");
				}
				if (!r5Allowed && (ARITHMETIC.has(op) || COMPARISON.has(op))) {
					const hit = [n.left, n.right].map(accessedName).find((a) => a && TIME_PROPS.has(a));
					if (hit) {
						report(n, "R5", `arithmetic on "${hit}": call a helper of units.ts`);
					}
				}
				break;
			}
			case SyntaxKind.Identifier: {
				const name = text(n);
				const parent = n.parent;
				const isMember =
					parent.kind === SyntaxKind.PropertyAccessExpression && parent.name === n;
				if (STORAGE_NAMES.has(name) && !isMember && !isPrefs) {
					report(n, "R7", `${name} outside state/prefs.ts`);
				}
				break;
			}
		}
		n.forEachChild(visit);
	};

	function checkImportGraph(n: Node, spec: string): void {
		if (!spec.startsWith(".")) return;
		const target = posix.normalize(posix.join(posix.dirname(rel), spec));
		if (target.startsWith("..")) return;
		const targetFolder = target.split("/")[0];
		if (targetFolder === folder || targetFolder.includes(".")) return;
		if (!(targetFolder in MAY_IMPORT) && targetFolder !== "fixtures") return;
		const allowed = MAY_IMPORT[folder];
		if (allowed === undefined || allowed.includes("*")) return;
		if (!allowed.includes(targetFolder)) {
			report(n, "R8", `editor/${folder} must not import editor/${targetFolder}`);
		}
	}

	visit(sf);
	return findings;
}

function scan(root: string, configFile: string): Finding[] {
	const cwd = root.split("\\").join("/");
	const api = new API({ cwd });
	try {
		const snapshot = api.updateSnapshot({ openProject: configFile.split("\\").join("/") });
		const project = snapshot.getProjects()[0];
		const findings: Finding[] = [];
		for (const file of project.program.getSourceFileNames()) {
			const rel = editorRelative(file);
			if (rel === null || file.includes("/node_modules/")) continue;
			const fixture = file.includes("/thin-client-fixtures/");
			if (!fixture && isExempt(rel)) continue;
			const sf = project.program.getSourceFile(file);
			if (sf === undefined) continue;
			findings.push(
				...checkFile(sf, rel, relative(cwd, file).split("\\").join("/")),
			);
		}
		return findings;
	} finally {
		api.close();
	}
}

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = join(here, "..");

function format(f: Finding): string {
	return `${f.file}:${f.line}  ${f.rule}  ${f.message}`;
}

// Every fixture directory is named <rule>-fail or <rule>-pass. A fail fixture
// must produce its rule and a pass fixture nothing.
function checkFixtures(): string[] {
	const dir = join(here, "thin-client-fixtures");
	const findings = scan(dir, join(dir, "tsconfig.json"));
	const problems: string[] = [];
	const names = new Set(findings.map((f) => f.file.split("/")[0]));
	const cases = findings.length >= 0 ? listCases(dir) : [];
	for (const name of cases) {
		const [rule, kind] = name.split("-");
		const mine = findings.filter((f) => f.file.split("/")[0] === name);
		if (kind === "fail" && !mine.some((f) => f.rule === rule.toUpperCase())) {
			problems.push(`fixture ${name} should fail ${rule.toUpperCase()} and did not`);
		}
		if (kind === "pass" && mine.length > 0) {
			problems.push(`fixture ${name} should pass but reports: ${mine.map(format).join("; ")}`);
		}
	}
	void names;
	return problems;
}

function listCases(dir: string): string[] {
	const out: string[] = [];
	for (const entry of new Bun.Glob("*-{fail,pass}").scanSync({ cwd: dir, onlyFiles: false })) {
		out.push(entry);
	}
	return out.sort();
}

const tree = scan(webRoot, join(webRoot, "tsconfig.json"));
for (const f of tree) console.error(format(f));
const fixtureProblems = checkFixtures();
for (const p of fixtureProblems) console.error(p);

if (tree.length > 0 || fixtureProblems.length > 0) {
	console.error(
		`check-thin-client: ${tree.length} finding(s), ${fixtureProblems.length} fixture problem(s)`,
	);
	process.exit(1);
}
console.log("check-thin-client: the tree is clean and every fixture behaves");
