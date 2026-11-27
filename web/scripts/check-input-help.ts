// Runs under Node (D-115 fallback): `typescript/unstable/sync` crashes under Bun's `child_process` on Linux.
// Universal input help guard (epic 112, SDD-012 §23.4):
//
//   bun run check:input-help
//
// Verifies that every user-editable <Input>, <Textarea>, <input>, and <textarea>
// in resources/js/**/*.tsx (excluding tests, generated files, fixtures, ui/
// primitives, readOnly copy boxes, and non-text input types) provides:
//   1. A `placeholder` attribute (for text-like inputs and textareas).
//   2. Contextual help via an adjacent <FieldLabel>, <FieldHelpTip>, <FieldHint>,
//      <Tooltip>, <HelpCard>, `aria-describedby`, or `title`.

import { dirname, join, relative } from "node:path";
import { fileURLToPath } from "node:url";
import { SyntaxKind } from "typescript/unstable/ast";
import { API } from "typescript/unstable/sync";

type Node = import("typescript/unstable/ast").Node;
type SourceFile = import("typescript/unstable/ast").SourceFile;
type JsxAttributes = import("typescript/unstable/ast").JsxAttributes;

interface Finding {
	file: string;
	line: number;
	rule: "IH1" | "IH2";
	message: string;
}

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = join(here, "..");

const SKIP_TYPES = new Set([
	"file",
	"hidden",
	"checkbox",
	"radio",
	"range",
	"color",
	"submit",
	"button",
	"reset",
	"image",
]);

const NO_PLACEHOLDER_TYPES = new Set([
	"date",
	"datetime-local",
	"time",
	"month",
	"week",
]);

const HELP_COMPONENTS = new Set([
	"FieldLabel",
	"FieldHelpTip",
	"FieldHint",
	"HelpCard",
	"Tooltip",
]);

function isExempt(file: string): boolean {
	return (
		/\.test\.tsx?$/.test(file) ||
		file.includes("/generated/") ||
		file.includes("/editor/fixtures/") ||
		file.includes("/components/ui/")
	);
}

function tagNameOf(node: Node): string | null {
	if (node.kind === SyntaxKind.JsxOpeningElement || node.kind === SyntaxKind.JsxSelfClosingElement) {
		if (node.tagName.kind === SyntaxKind.Identifier) return node.tagName.text;
	}
	return null;
}

function attrValue(attrs: JsxAttributes, name: string): { present: boolean; literal: string | null } {
	for (const prop of attrs.properties) {
		if (prop.kind !== SyntaxKind.JsxAttribute) continue;
		if (prop.name.kind !== SyntaxKind.Identifier || prop.name.text !== name) continue;
		if (prop.initializer === undefined) return { present: true, literal: "" };
		if (prop.initializer.kind === SyntaxKind.StringLiteral) {
			return { present: true, literal: prop.initializer.text };
		}
		if (
			prop.initializer.kind === SyntaxKind.JsxExpression &&
			prop.initializer.expression !== undefined &&
			prop.initializer.expression.kind === SyntaxKind.StringLiteral
		) {
			return { present: true, literal: prop.initializer.expression.text };
		}
		return { present: true, literal: null };
	}
	return { present: false, literal: null };
}

function hasSpreadWithKey(attrs: JsxAttributes, sf: SourceFile, key: string): boolean {
	const text = sf.text;
	for (const prop of attrs.properties) {
		if (prop.kind !== SyntaxKind.JsxSpreadAttribute) continue;
		if (prop.expression.kind === SyntaxKind.Identifier) {
			const ident = prop.expression.text;
			const pattern = new RegExp(`\\b${ident}\\s*=\\s*\\{[\\s\\S]{0,1200}?["']?${key}["']?\\s*:`);
			if (pattern.test(text)) return true;
		}
	}
	return false;
}

function subtreeHasHelpComponent(node: Node): boolean {
	let found = false;
	const walk = (n: Node): void => {
		if (found) return;
		const tag = tagNameOf(n);
		if (tag !== null && HELP_COMPONENTS.has(tag)) {
			found = true;
			return;
		}
		n.forEachChild(walk);
	};
	walk(node);
	return found;
}

function enclosingHasHelp(node: Node): boolean {
	let cur: Node | undefined = node.parent;
	for (let depth = 0; depth < 6 && cur !== undefined; depth++) {
		if (
			cur.kind === SyntaxKind.JsxElement ||
			cur.kind === SyntaxKind.JsxFragment ||
			cur.kind === SyntaxKind.FunctionDeclaration ||
			cur.kind === SyntaxKind.ArrowFunction
		) {
			if (subtreeHasHelpComponent(cur)) return true;
		}
		if (cur.kind === SyntaxKind.FunctionDeclaration || cur.kind === SyntaxKind.ArrowFunction) {
			break;
		}
		cur = cur.parent;
	}
	return false;
}

function scanFile(sf: SourceFile, fileLabel: string): Finding[] {
	const findings: Finding[] = [];
	const lineOf = (n: Node): number =>
		sf.getLineAndCharacterOfPosition(n.getStart(sf)).line + 1;

	const visit = (n: Node): void => {
		if (n.kind === SyntaxKind.JsxOpeningElement || n.kind === SyntaxKind.JsxSelfClosingElement) {
			const tag = tagNameOf(n);
			if (tag === "Input" || tag === "Textarea" || tag === "input" || tag === "textarea") {
				const attrs = n.attributes;
				const readOnly = attrValue(attrs, "readOnly");
				const typeAttr = attrValue(attrs, "type");
				const inputType = typeAttr.literal?.toLowerCase() ?? "";

				if (!readOnly.present && !SKIP_TYPES.has(inputType)) {
					const isTextarea = tag === "Textarea" || tag === "textarea";
					const needsPlaceholder = isTextarea || !NO_PLACEHOLDER_TYPES.has(inputType);

					if (needsPlaceholder) {
						const ph = attrValue(attrs, "placeholder");
						const spreadPh = hasSpreadWithKey(attrs, sf, "placeholder");
						if (!ph.present && !spreadPh) {
							findings.push({
								file: fileLabel,
								line: lineOf(n),
								rule: "IH1",
								message: `<${tag}> is missing a localized placeholder`,
							});
						}
					}

					const describedBy = attrValue(attrs, "aria-describedby");
					const spreadDescribedBy = hasSpreadWithKey(attrs, sf, "aria-describedby");
					const titleAttr = attrValue(attrs, "title");
					const hasHelp =
						describedBy.present ||
						spreadDescribedBy ||
						titleAttr.present ||
						enclosingHasHelp(n);

					if (!hasHelp) {
						findings.push({
							file: fileLabel,
							line: lineOf(n),
							rule: "IH2",
							message: `<${tag}> has no contextual help (FieldLabel, FieldHelpTip, FieldHint, HelpCard, Tooltip, or aria-describedby)`,
						});
					}
				}
			}
		}
		n.forEachChild(visit);
	};

	visit(sf);
	return findings;
}

const cwd = webRoot.split("\\").join("/");
const api = new API({ cwd });
const findings: Finding[] = [];
let scanned = 0;

try {
	const snapshot = api.updateSnapshot({
		openProject: join(webRoot, "tsconfig.json").split("\\").join("/"),
	});
	const project = snapshot.getProjects()[0];
	for (const file of project.program.getSourceFileNames()) {
		if (file.includes("/node_modules/") || !file.includes("/resources/js/")) continue;
		if (!file.endsWith(".tsx") || isExempt(file)) continue;
		const sf = project.program.getSourceFile(file);
		if (sf === undefined) continue;
		scanned++;
		findings.push(...scanFile(sf, relative(cwd, file).split("\\").join("/")));
	}
} finally {
	api.close();
}

if (findings.length > 0) {
	for (const f of findings) {
		console.error(`${f.file}:${f.line}  ${f.rule}  ${f.message}`);
	}
	console.error(`check-input-help: ${findings.length} finding(s) across ${scanned} files`);
	process.exit(1);
}

console.log(`check-input-help: OK (${scanned} files checked, every editable input has placeholder and contextual help)`);
