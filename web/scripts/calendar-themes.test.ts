import { readFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { describe, expect, test } from "vitest";

import { THEMES } from "../resources/js/pages/Publish/dayTypes";
import { THEME_CLASSES } from "../resources/js/pages/Publish/theme";
import { evaluate } from "./check-contrast";

// A theme of a day of the publishing calendar (SDD-020 section 30A.1) is a family of the design system whose text on its
// soft surface the contrast check measures, so a theme can never be drawn in a pair nobody checked (docs/design-system.md
// section 2.6, "Calendar themes").
const css = readFileSync(
	join(dirname(fileURLToPath(import.meta.url)), "../resources/css/app.css"),
	"utf8",
);

const PAIRS: Record<(typeof THEMES)[number], { fg: string; bg: string }> = {
	blue: { fg: "color-accent-text", bg: "color-accent-soft" },
	green: { fg: "color-success-text", bg: "color-success-soft" },
	amber: { fg: "color-warning-text", bg: "color-warning-soft" },
	red: { fg: "color-danger-text", bg: "color-danger-soft" },
	teal: { fg: "color-agent-text", bg: "color-agent-soft" },
	gray: { fg: "color-text", bg: "color-surface-sunken" },
};

describe("the themes of a day", () => {
	const rows = evaluate(css);

	for (const name of THEMES) {
		test(`${name}: its text on its surface passes 4.5:1 in both themes of the app`, () => {
			const { fg, bg } = PAIRS[name];
			const named = rows.filter((r) => r.name === `${fg} on ${bg}`);

			expect(named.map((r) => r.theme).sort()).toEqual(["dark", "light"]);

			for (const row of named) {
				expect(row.problems, `${name} in ${row.theme}`).toEqual([]);
				expect(row.got).toBeGreaterThanOrEqual(4.5);
			}
		});

		test(`${name}: the page draws exactly that pair`, () => {
			const { fg, bg } = PAIRS[name];
			const classes = THEME_CLASSES[name].surface.split(" ");

			expect(classes).toEqual([
				`bg-${bg.replace("color-", "")}`,
				`text-${fg.replace("color-", "")}`,
			]);
		});
	}
});
