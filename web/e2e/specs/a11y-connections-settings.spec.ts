import AxeBuilder from "@axe-core/playwright";

import { horizontalOverflow, undersizedTargets } from "../support/wcag";
import { expect, test } from "./helpers";

// Epic 84: the connections, settings, webhooks and error screens at 1440, 1280 and 390 px in both themes. axe
// finds nothing at WCAG 2.2 AA, nothing scrolls sideways and no target is too small. The account screens
// are measured in account-first-run.spec.ts, on the server that has no account yet.
const TAGS = ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"];

const WIDTHS = [
	{ width: 1440, height: 900 },
	{ width: 1280, height: 800 },
	{ width: 390, height: 844 },
] as const;

const PAGES = [
	{ path: "/connections", heading: "Connections" },
	{ path: "/settings", heading: "Settings" },
	{ path: "/webhooks", heading: "Webhooks" },
	{ path: "/settings/no-such-page", heading: /Not Found/ },
] as const;

test.describe("a11y-connections-settings", () => {
	test.describe.configure({ timeout: 120_000 });

	for (const theme of ["light", "dark"] as const) {
		for (const size of WIDTHS) {
			test(`the screens of epic 84 are clean at ${size.width} px in ${theme}`, async ({
				page,
			}) => {
				await page.addInitScript(
					(th) => localStorage.setItem("theme", th),
					theme,
				);
				await page.setViewportSize(size);
				const found: string[] = [];
				for (const { path, heading } of PAGES) {
					await page.goto(path);
					await expect(
						page.getByRole("heading", { level: 1, name: heading }),
					).toBeVisible();
					await expect
						.poll(() =>
							page.evaluate(
								() =>
									document
										.getAnimations()
										.filter(
											(a) =>
												a.playState === "running" &&
												a.effect?.getComputedTiming().iterations !== Infinity,
										).length,
							),
						)
						.toBe(0);
					const { violations } = await new AxeBuilder({ page })
						.withTags(TAGS)
						.analyze();
					for (const v of violations) found.push(`${path}: ${v.id}`);
					const overflow = await horizontalOverflow(page);
					if (overflow) found.push(`${path}: ${overflow}`);
					for (const small of await undersizedTargets(page)) {
						found.push(`${path}: ${small.target} ${small.size}`);
					}
				}
				expect(found).toEqual([]);
			});
		}
	}
});
