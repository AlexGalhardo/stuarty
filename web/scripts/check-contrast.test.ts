import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, it } from "vitest";
import { evaluate } from "./check-contrast";

const css = readFileSync(
	join(dirname(fileURLToPath(import.meta.url)), "../resources/css/app.css"),
	"utf8",
);

describe("check-contrast", () => {
	it("passes every pair of the real stylesheet in both themes", () => {
		const failing = evaluate(css).filter((r) => r.problems.length > 0);
		expect(failing).toEqual([]);
	});

	it("detects a token pair below 4.5:1", () => {
		// muted-foreground nearly as light as the background in the light theme.
		const bad = css.replace(
			/(--muted-foreground:\s*)oklch\([^)]*\)/,
			"$1oklch(0.85 0 0)",
		);
		expect(bad).not.toBe(css);
		const failing = evaluate(bad).filter((r) =>
			r.problems.some((p) => p.startsWith("below")),
		);
		expect(failing.length).toBeGreaterThan(0);
		expect(failing.every((r) => r.name.includes("muted-foreground"))).toBe(true);
	});
});
