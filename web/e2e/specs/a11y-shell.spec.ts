import AxeBuilder from "@axe-core/playwright";

import { horizontalOverflow, undersizedTargets } from "../support/wcag";
import { expect, test } from "./helpers";

// Epic 75 (SDD-012 section 23.4): axe over the shell, with the notification center open and, on a phone, the
// drawer open, at 1440, 1280 and 390 px in both themes; nothing scrolls sideways, no target is too small.
const TAGS = ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"];

const WIDTHS = [
	{ width: 1440, height: 900 },
	{ width: 1280, height: 800 },
	{ width: 390, height: 844 },
] as const;

test.describe("a11y-shell", () => {
	test.describe.configure({ timeout: 120_000 });

	for (const theme of ["light", "dark"] as const) {
		for (const size of WIDTHS) {
			test(`axe finds nothing in the shell at ${size.width} px in ${theme}`, async ({
				page,
			}) => {
				await page.addInitScript(
					(th) => localStorage.setItem("theme", th),
					theme,
				);
				await page.setViewportSize(size);
				await page.goto("/");
				await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
				await expect(page.getByTestId("scene-bar")).toBeVisible();
				const found: string[] = [];
				const scan = async (what: string): Promise<void> => {
					// A sheet still fading in has text halfway to its colour, which axe reads as low contrast:
					// wait for the animations that end (the spinner of a working agent never does).
					await page.evaluate(() =>
						Promise.race([
							Promise.all(
								document
									.getAnimations()
									.filter(
										(a) =>
											a.effect?.getComputedTiming().iterations !== Infinity,
									)
									.map((a) => a.finished.catch(() => undefined)),
							),
							new Promise((resolve) => setTimeout(resolve, 3000)),
						]),
					);
					const { violations } = await new AxeBuilder({ page })
						.withTags(TAGS)
						.analyze();
					for (const v of violations) found.push(`${what}: ${v.id}`);
				};
				await scan("page");
				const overflow = await horizontalOverflow(page);
				if (overflow) found.push(`page: ${overflow}`);
				for (const small of await undersizedTargets(page)) {
					found.push(`page: ${small.target} ${small.size}`);
				}

				if (size.width < 768) {
					await page.getByTestId("menu-toggle").click();
					await expect(page.getByTestId("sidebar-drawer")).toBeVisible();
					await scan("drawer");
					await page.keyboard.press("Escape");
					await expect(page.getByTestId("sidebar-drawer")).toHaveCount(0);
				}

				await page.getByTestId("bell").click();
				await expect(page.getByTestId("notification-center")).toBeVisible();
				await scan("center");
				expect(found).toEqual([]);
			});
		}
	}
});
