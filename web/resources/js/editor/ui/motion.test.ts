import { readdirSync, readFileSync } from "node:fs";
import { join } from "node:path";
import { describe, expect, test } from "vitest";

// SDD-012 section 17 item 5: the editor animates only through the two classes
// of app.css, and every one of them is off under prefers-reduced-motion.
const css = readFileSync(
	new URL("../../../css/app.css", import.meta.url),
	"utf8",
).replace(/\r\n/g, "\n");

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

// A Tailwind class that moves something: transition, animate-*, duration-*.
const CLASS_TOKEN =
	/["'`\s](transition(-[\w[\]-]+)?|animate-[\w-]+|duration-[\w[\]-]+)(?=["'`\s])/;

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
			expect(rule).not.toMatch(new RegExp(`\b${property}\b[,;]`));
		}
	});

	test("editor components take no other transition or animation", () => {
		const root = new URL("../", import.meta.url);
		const offenders: string[] = [];
		const walk = (dir: string): void => {
			for (const entry of readdirSync(dir, { withFileTypes: true })) {
				const path = join(dir, entry.name);
				if (entry.isDirectory()) walk(path);
				else if (
					/\.tsx$/.test(entry.name) &&
					!entry.name.endsWith(".test.tsx")
				) {
					const text = readFileSync(path, "utf8")
						.replace(/\/\*[\s\S]*?\*\//g, "")
						.replace(/\/\/.*$/gm, "");
					if (CLASS_TOKEN.test(text)) offenders.push(path);
				}
			}
		};
		walk(root.pathname.replace(/^\/([A-Za-z]:)/, "$1"));
		expect(offenders).toEqual([]);
	});
});
