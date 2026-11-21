import AxeBuilder from "@axe-core/playwright";

import { expect, test } from "./helpers";

// RF-310 and NFR-065 (epic 07): axe over the dashboard home and every section,
// in both languages and both themes, and the home with the sidebar collapsed.
const PAGES = [
	"/",
	"/projects",
	"/edit-video",
	"/edit-image",
	"/write",
	"/visual-assets",
	"/video-assets",
	"/sound-assets",
	"/publish",
	"/skills",
	"/prompts",
	"/memory",
	"/connections",
] as const;

const TAGS = ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"];

test.describe("a11y-dashboard", () => {
	// Twelve pages, each scanned by axe: Firefox needs more than the 30 s default.
	test.describe.configure({ timeout: 120_000 });

	for (const lang of ["en", "pt-BR"] as const) {
		for (const theme of ["light", "dark"] as const) {
			test(`axe finds nothing on the dashboard in ${theme} in ${lang}`, async ({
				page,
			}) => {
				await page.addInitScript(
					([l, th]) => {
						localStorage.setItem("vv.lang", l);
						localStorage.setItem("theme", th);
					},
					[lang, theme],
				);

				const found: string[] = [];
				for (const path of PAGES) {
					await page.goto(path);
					await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
					const { violations } = await new AxeBuilder({ page })
						.withTags(TAGS)
						.analyze();
					for (const v of violations) found.push(`${path}: ${v.id}`);
				}
				expect(found).toEqual([]);
			});
		}
	}

	test("axe finds nothing with the sidebar collapsed", async ({ page }) => {
		await page.addInitScript(() =>
			localStorage.setItem("sidebar", "collapsed"),
		);
		await page.goto("/");
		await expect(page.getByTestId("sidebar")).toHaveAttribute(
			"data-state",
			"collapsed",
		);
		const { violations } = await new AxeBuilder({ page })
			.withTags(TAGS)
			.analyze();
		expect(violations.map((v) => v.id)).toEqual([]);
	});
});
