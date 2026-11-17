import AxeBuilder from "@axe-core/playwright";

import { expect, seedProject, test } from "./helpers";

// TASK-0285 (SDD-012 section 6.4): the material region has a vertical rail of
// 40 px with one icon tab per panel; a tab is drawn only when its command is in
// the registry, and Alt+1 hides the panel but keeps the rail.
test.describe("library tab rail", () => {
	test.use({ viewport: { width: 1600, height: 900 } });

	test("the rail is a vertical tablist with roving keys", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		const rail = page.getByRole("tablist", { name: "Material panels" });
		await expect(rail).toHaveAttribute("aria-orientation", "vertical");
		expect((await rail.boundingBox())?.width).toBeCloseTo(40, 0);
		const media = page.getByRole("tab", { name: "Media" });
		const history = page.getByRole("tab", { name: "History" });
		await expect(media).toHaveAttribute("aria-selected", "true");
		await expect(page.getByRole("region", { name: "Media" })).toBeVisible();

		await media.focus();
		await page.keyboard.press("ArrowDown");
		await expect(history).toBeFocused();
		await expect(history).toHaveAttribute("aria-selected", "true");
		await expect(page.getByRole("region", { name: "History" })).toBeVisible();
		await page.keyboard.press("ArrowUp");
		await expect(media).toBeFocused();
		await page.keyboard.press("End");
		await expect(history).toBeFocused();
		await page.keyboard.press("Home");
		await expect(media).toBeFocused();
		await expect(media).toHaveAttribute("aria-selected", "true");
		// A rail tab is one tab stop: the other one is not in the tab order.
		await expect(history).toHaveAttribute("tabindex", "-1");
	});

	test("Alt+1 hides the whole panel and keeps the rail", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		const aside = page.locator("aside[data-landmark='2']");
		await expect(aside).toBeVisible();
		expect((await aside.boundingBox())?.width).toBeCloseTo(320, 0);
		await page.keyboard.press("Alt+1");
		await expect(page.getByRole("region", { name: "Media" })).toHaveCount(0);
		await expect(page.getByRole("tab", { name: "Media" })).toBeVisible();
		await expect.poll(async () => (await aside.boundingBox())?.width).toBe(40);
		// A click on a tab brings the panel back.
		await page.getByRole("tab", { name: "History" }).click();
		await expect(page.getByRole("region", { name: "History" })).toBeVisible();
		await page.keyboard.press("Alt+1");
		await expect(page.getByRole("region", { name: "Media" })).toBeVisible();
	});

	test("a tab whose command is not in the registry is not drawn", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.route("**/api/v1/commands*", async (route) => {
			const response = await route.fetch();
			const body = (await response.json()) as {
				commands: { name: string }[];
			};
			await route.fulfill({
				response,
				json: {
					...body,
					commands: body.commands.filter((c) => c.name !== "clip.add"),
				},
			});
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.getByRole("tab", { name: "History" })).toBeVisible();
		await expect(page.getByRole("tab", { name: "Media" })).toHaveCount(0);
		await page.unrouteAll({ behavior: "ignoreErrors" });
	});

	for (const lang of ["en", "pt-BR"] as const) {
		for (const theme of ["light", "dark"] as const) {
			test(`axe finds nothing in ${theme} in ${lang}`, async ({
				page,
				request,
			}) => {
				await page.addInitScript(
					([l, th]) => {
						localStorage.setItem("vv.lang", l);
						if (th === "light") localStorage.setItem("theme", "light");
					},
					[lang, theme],
				);
				const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
				await page.goto(`/editor/${seeded.id}`);
				await expect(page.getByRole("tablist")).toBeVisible();
				const { violations } = await new AxeBuilder({ page })
					.include("aside[data-landmark='2']")
					.withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
					.analyze();
				expect(violations.map((v) => v.id)).toEqual([]);
			});
		}
	}
});
