import AxeBuilder from "@axe-core/playwright";
import { expect, listProjects, seedProject, test } from "./helpers";

// TASK-0293 (SDD-012 sections 2.1 and 6.4): the projects page is a grid of
// cards, each with a menu of Rename and Delete.
test.describe("projects grid", () => {
	test("a card shows the name, the size and the way into the editor", async ({
		page,
		request,
	}) => {
		const label = `Grid ${Date.now()}`;
		const seeded = await seedProject(request, {
			name: label,
			tracks: 1,
			perTrack: 1,
		});
		await page.goto("/projects");
		const card = page.locator(`[data-project-card='${seeded.id}']`);
		await expect(card).toContainText(label);
		await expect(card).toContainText(/1080 × 1920 px/);
		await expect(
			card.getByRole("link", { name: new RegExp(`Open.*${label}`) }),
		).toHaveAttribute("href", `/editor/${seeded.id}`);
		await card.getByRole("button", { name: /Actions for project/ }).click();
		await expect(page.getByRole("menuitem")).toHaveText(["Rename", "Delete"]);
	});

	test("Rename sends project.rename and the card shows the new name", async ({
		page,
		request,
	}) => {
		const label = `Rename ${Date.now()}`;
		const seeded = await seedProject(request, { name: label, tracks: 0 });
		await page.goto("/projects");
		const card = page.locator(`[data-project-card='${seeded.id}']`);
		await card.getByRole("button", { name: /Actions for project/ }).click();
		await page.getByRole("menuitem", { name: "Rename" }).click();
		const field = page.getByRole("dialog").getByLabel("Name");
		await expect(field).toHaveValue(label);
		await field.fill(`${label} renamed`);
		await page.getByRole("button", { name: "Save" }).click();
		await expect(card).toContainText(`${label} renamed`);
		const stored = (await listProjects(request)).find(
			(p) => p.id === seeded.id,
		);
		expect(stored?.name).toBe(`${label} renamed`);
	});

	test("Delete asks first: cancel keeps the project, confirm moves it to the trash", async ({
		page,
		request,
	}) => {
		const label = `Delete ${Date.now()}`;
		const seeded = await seedProject(request, { name: label, tracks: 0 });
		await page.goto("/projects");
		const card = page.locator(`[data-project-card='${seeded.id}']`);
		const open = async (): Promise<void> => {
			await card.getByRole("button", { name: /Actions for project/ }).click();
			await page.getByRole("menuitem", { name: "Delete" }).click();
		};
		await open();
		const dialog = page.getByRole("alertdialog");
		await expect(dialog).toContainText("Delete this project?");
		await dialog.getByRole("button", { name: "Cancel" }).click();
		await expect(card).toBeVisible();

		await open();
		await dialog.getByRole("button", { name: "Move to trash" }).click();
		await expect(card).toHaveCount(0);
		await expect(
			page.locator(`[data-trashed-id='${seeded.id}']`),
		).toBeVisible();
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
						localStorage.setItem("theme", th);
					},
					[lang, theme],
				);
				await seedProject(request, { tracks: 1, perTrack: 1 });
				await page.goto("/projects");
				await expect(page.locator("[data-project-card]").first()).toBeVisible();
				const { violations } = await new AxeBuilder({ page })
					.withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
					.analyze();
				expect(violations.map((v) => v.id)).toEqual([]);
			});
		}
	}
});
