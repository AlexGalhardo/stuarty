import { readdirSync, readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, test } from "vitest";

const web = join(dirname(fileURLToPath(import.meta.url)), "..");
const appCss = readFileSync(join(web, "resources/css/app.css"), "utf8");

// Every component of the editor, as text.
const editor = join(web, "resources/js/editor");
const sources = readdirSync(editor, { recursive: true, encoding: "utf8" })
	.filter((path) => path.endsWith(".tsx"))
	.map((path) => [path, readFileSync(join(editor, path), "utf8")] as const);

// SDD-012 section 17 item 5: the editor animates only through the two classes
// of app.css, and every one of them is off under prefers-reduced-motion.
const css = appCss.replace(/\r\n/g, "\n");

// A Tailwind class that moves something: transition, animate-*, duration-*.
const CLASS_TOKEN =
	/["'`\s](transition(-[\w[\]-]+)?|animate-[\w-]+|duration-[\w[\]-]+)(?=["'`\s])/;

function block(start: string): string {
	const at = css.indexOf(start);
	expect(at).toBeGreaterThanOrEqual(0);
	let depth = 0;
	for (let i = css.indexOf("{", at); i < css.length; i++) {
		if (css[i] === "{") depth++;
		if (css[i] === "}" && --depth === 0) return css.slice(at, i + 1);
	}
	throw new Error(`unterminated block ${start}`);
}

describe("reduced motion", () => {
	const reduced = block("@media (prefers-reduced-motion: reduce)");

	test("turns every transition off at 0s and animations to one instant pass", () => {
		expect(reduced).toMatch(/transition-duration:\s*0s !important/);
		expect(reduced).toMatch(/transition-delay:\s*0s !important/);
		expect(reduced).toMatch(/animation-duration:\s*0\.01ms !important/);
		expect(reduced).toMatch(/animation-iteration-count:\s*1 !important/);
		expect(reduced).toMatch(/\*,\s*::before,\s*::after/);
	});

	test("comes after the classes that move, so it wins at equal weight", () => {
		const media = css.indexOf("@media (prefers-reduced-motion: reduce)");
		expect(css.indexOf(".ed-motion {")).toBeLessThan(media);
		expect(css.indexOf(".ed-highlight {")).toBeLessThan(media);
	});
});

describe("interface motion", () => {
	test("uses the duration and easing tokens, never a literal time", () => {
		for (const rule of [block(".ed-motion {"), block(".ed-highlight {")]) {
			expect(rule).toMatch(/var\(--duration-(base|slow)\)/);
			expect(rule).toMatch(/var\(--ease-standard\)/);
			expect(rule).not.toMatch(/\d+m?s\b/);
		}
	});

	test("never transitions position or size", () => {
		const rule = block(".ed-motion {");
		for (const property of ["left", "top", "width", "height", "transform"]) {
			expect(rule).not.toMatch(new RegExp(`\\b${property}\\b[,;]`));
		}
	});

	test("editor components take no other transition or animation", () => {
		const offenders = sources
			.filter(([path]) => !path.endsWith(".test.tsx"))
			.filter(([, source]) =>
				CLASS_TOKEN.test(
					source.replace(/\/\*[\s\S]*?\*\//g, "").replace(/\/\/.*$/gm, ""),
				),
			)
			.map(([path]) => path);
		expect(sources.length).toBeGreaterThan(50);
		expect(offenders).toEqual([]);
	});
});
