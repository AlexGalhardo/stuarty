import { expect, test } from "./helpers";

// VV-438: on a window 800 px high the list of the sidebar scrolls, and the item of the page being read stayed below
// the fold, so the sidebar showed no current page.
test.describe("sidebar on a short window", () => {
	test.use({ viewport: { width: 1280, height: 800 } });

	test("the item of the current page is inside the visible list", async ({
		page,
	}) => {
		await page.goto("/webhooks");
		const current = page
			.getByRole("navigation", { name: "Main navigation" })
			.locator('[aria-current="page"]');
		await expect(current).toBeVisible();
		await expect
			.poll(() =>
				page.evaluate(() => {
					const nav = document.querySelector(
						'nav[aria-label="Main navigation"]',
					);
					const link = nav?.querySelector('[aria-current="page"]');
					if (!link || !nav) return false;
					const a = link.getBoundingClientRect();
					const b = nav.getBoundingClientRect();
					return a.top >= b.top - 1 && a.bottom <= b.bottom + 1;
				}),
			)
			.toBe(true);
	});
});
