// Verifies 100% alignment between canonical error (E_*), lint (L_*), and export-check (X_*)
// codes in docs/sdds/000-shared-definitions.md + core registries and both language catalogues
// (en.json and pt-BR.json), with zero untranslated English duplicates in pt-BR (epic 113).

import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";

import { type Catalog, flatten } from "./i18n-catalog.ts";

const here = dirname(fileURLToPath(import.meta.url));
const webRoot = join(here, "..");
const repoRoot = join(webRoot, "..");

function read(path: string): string {
	return readFileSync(path, "utf8");
}

function loadCatalog(name: string) {
	return flatten(JSON.parse(read(join(webRoot, "resources/lang", name))) as Catalog);
}

function extractMatches(text: string, pattern: RegExp): string[] {
	return [...new Set([...text.matchAll(pattern)].map((m) => m[0]))].sort();
}

const sddDir = join(repoRoot, "docs/sdds");
const sdd000 = read(join(sddDir, "000-shared-definitions.md"));
const sdd002Name = readdirSync(sddDir).find((n) => n.startsWith("002-") && n.endsWith(".md"));
if (sdd002Name === undefined) {
	console.error("check-error-i18n: could not locate docs/sdds/002-*.md");
	process.exit(1);
}
const sdd002 = read(join(sddDir, sdd002Name));
const allSdds = readdirSync(sddDir)
	.filter((n) => n.endsWith(".md"))
	.map((n) => read(join(sddDir, n)))
	.join("\n");

const eCodes = [
	...new Set([
		...extractMatches(`${sdd000}\n${sdd002}`, /\bE_[A-Z0-9_]+\b/g),
		"E_NETWORK",
	]),
].sort();

const lCodes = extractMatches(allSdds, /\bL_[A-Z0-9_]+\b/g);
const xCodes = extractMatches(sdd000, /\bX_[A-Z0-9_]+\b/g);

const en = loadCatalog("en.json");
const pt = loadCatalog("pt-BR.json");

const problems: string[] = [];

function checkKey(key: string): void {
	const enLeaf = en.get(key);
	const ptLeaf = pt.get(key);
	if (enLeaf === undefined) {
		problems.push(`missing in en.json: ${key}`);
	}
	if (ptLeaf === undefined) {
		problems.push(`missing in pt-BR.json: ${key}`);
	}
	if (
		enLeaf !== undefined &&
		ptLeaf !== undefined &&
		enLeaf.texts.join("\u0000") === ptLeaf.texts.join("\u0000")
	) {
		problems.push(`untranslated in pt-BR.json (identical to en.json): ${key}`);
	}
}

for (const code of eCodes) {
	checkKey(`ui.editor.errors.${code}`);
	checkKey(`ui.errors.message.${code}`);
}

for (const code of lCodes) {
	checkKey(`ui.lint.message.${code}`);
}

for (const code of xCodes) {
	checkKey(`ui.export_check.message.${code}`);
}

// Also ensure every reason / reason_hint / hint key under ui.errors.* is present and translated in pt-BR.
for (const key of en.keys()) {
	if (
		key.startsWith("ui.errors.") ||
		key.startsWith("ui.lint.") ||
		key.startsWith("ui.export_check.") ||
		key.startsWith("ui.jobs.phase.")
	) {
		checkKey(key);
	}
}

if (problems.length > 0) {
	for (const p of problems) console.error(`check-error-i18n: ${p}`);
	console.error(`check-error-i18n: ${problems.length} problem(s) found`);
	process.exit(1);
}

console.log(
	`check-error-i18n: OK (${eCodes.length} E_* codes, ${lCodes.length} L_* lint codes, ${xCodes.length} X_* export checks fully localized in en and pt-BR)`,
);
