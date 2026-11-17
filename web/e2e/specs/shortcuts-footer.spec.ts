import AxeBuilder from "@axe-core/playwright";
import { expect, seedProject, test } from "./helpers";

// TASK-0294 (SDD-012 sections 6.4 and 18.2): the footer of the shortcuts dialog
// has the language, the theme and the version of the service.
test.describe("shortcuts dialog footer", () => {
	test("the version is the one the service reports, and theme and language work from the dialog", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.locator("body").press("?");
		const dialog = page.getByRole("dialog", { name: "Keyboard shortcuts" });
		await expect(dialog).toBeVisible();

		const system = await page.evaluate(async () => {
			const response = await fetch("/api/v1/system", {
				credentials: "same-origin",
			});
			return (await response.json()) as { version: string };
		});
		await expect(dialog.locator("[data-version]")).toHaveText(
			`Version ${system.version}`,
		);

		const dark = await page.evaluate(() =>
			document.documentElement.classList.contains("dark"),
		);
		await dialog
			.getByRole("button", {
				name: dark ? "Switch to light theme" : "Switch to dark theme",
			})
			.click();
		await expect
			.poll(() =>
				page.evaluate(() =>
					document.documentElement.classList.contains("dark"),
				),
			)
			.toBe(!dark);

		await dialog.getByRole("combobox", { name: "Language" }).click();
		await expect(page.getByRole("option", { name: /Português/ })).toBeVisible();
	});

	for (const lang of ["en", "pt-BR"] as const) {
		for (const theme of ["light", "dark"] as const) {
			test(`axe finds nothing in the dialog in ${theme} in ${lang}`, async ({
				page,
				request,
			}) => {
				await page.addInitScript(
					([l, th]) => {
						localStorage.setItem("vv.lang", l);
						localStorage.setItem("theme", th);
					},
					[lang, theme],
				);
				const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
				await page.goto(`/editor/${seeded.id}`);
				await expect(page.locator("[data-item-id]")).toHaveCount(1);
				await page.locator("body").press("?");
				const dialog = page.getByRole("dialog");
				await expect(dialog.locator("[data-version]")).toBeVisible();
				await page.evaluate(() =>
					Promise.all(
						document
							.getAnimations()
							.map((a) => a.finished.catch(() => undefined)),
					),
				);
				const { violations } = await new AxeBuilder({ page })
					.include("[role='dialog']")
					.withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
					.analyze();
				expect(violations.map((v) => v.id)).toEqual([]);
			});
		}
	}
});
