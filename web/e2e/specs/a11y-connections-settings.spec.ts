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
	{ path: "/settings/no-such-page", heading: "We could not find this page" },
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
					if (path === "/settings") {
						// Every section of the list is on the screen: none hides in a sideways scroll on a phone.
						const links = page
							.getByRole("navigation", { name: "Sections of the settings" })
							.getByRole("link");
						for (const link of await links.all()) {
							const box = await link.boundingBox();
							if (box === null || box.x < 0 || box.x + box.width > size.width) {
								found.push(
									`${path}: ${await link.textContent()} off the screen`,
								);
							}
						}
					}
				}
				expect(found).toEqual([]);
			});
		}
	}
});
