import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

async function htmlLang(page: Page): Promise<string | null> {
	return page.evaluate(() => document.documentElement.getAttribute("lang"));
}

async function langViolations(page: Page): Promise<string[]> {
	const { violations } = await new AxeBuilder({ page })
		.withRules(["html-has-lang", "html-lang-valid"])
		.analyze();
	return violations.map((v) => v.id);
}

// VV-169: the lang of <html> follows the active language on every page, after
// a switch and when the stored choice differs from the first rendering.
test.describe("html-lang", () => {
	test("a page follows a switch to pt-BR and back", async ({ page }) => {
		await page.goto("/projects");
		expect(await htmlLang(page)).toBe("en");
		await page.getByRole("combobox", { name: "Language" }).click();
		await page.getByRole("option", { name: "Português (Brasil)" }).click();
		await expect(
			page.getByRole("heading", { name: "Projetos", level: 1 }),
		).toBeVisible();
		await expect.poll(() => htmlLang(page)).toBe("pt-BR");
		expect(await langViolations(page)).toEqual([]);
		await page.getByRole("combobox", { name: "Idioma" }).click();
		await page.getByRole("option", { name: "English" }).click();
		await expect(
			page.getByRole("heading", { name: "Projects", level: 1 }),
		).toBeVisible();
		await expect.poll(() => htmlLang(page)).toBe("en");
	});

	test("a stored pt-BR choice reaches the first page of a visit", async ({
		page,
	}) => {
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await page.goto("/projects");
		await expect(
			page.getByRole("heading", { name: "Projetos", level: 1 }),
		).toBeVisible();
		await expect.poll(() => htmlLang(page)).toBe("pt-BR");
		expect(await langViolations(page)).toEqual([]);
	});

	test("the editor in pt-BR has the lang of the language", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.addInitScript(() => localStorage.setItem("vv.lang", "pt-BR"));
		await page.goto(`/editor/${seeded.id}`);
		await expect(
			page.getByRole("link", { name: "Voltar aos projetos" }),
		).toBeVisible();
		expect(await htmlLang(page)).toBe("pt-BR");
		expect(await langViolations(page)).toEqual([]);
	});
});
