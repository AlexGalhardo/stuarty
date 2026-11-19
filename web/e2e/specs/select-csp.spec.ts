import { expect, seedProject, test } from "./helpers";

// VV-236, VV-247 and VV-249: the language select (a Base UI Select) renders an
// inline <style> for its popup. Without the nonce of the page it breaks
// `style-src 'nonce-...'` and the browser reports `style-src-elem`. The fixture
// of helpers.ts fails every spec that sees a securitypolicyviolation, so this
// one only has to focus, open and use the select, on a page and in the editor.
test.describe("the language select and the content security policy", () => {
	test("focusing and opening the select of a page reports no violation", async ({
		page,
	}) => {
		await page.goto("/projects");
		const select = page.getByRole("combobox", { name: "Language" });
		await select.focus();
		await expect(select).toBeFocused();
		await select.click();
		await expect(page.getByRole("option", { name: "English" })).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(page.getByRole("option", { name: "English" })).toBeHidden();
	});

	test("the select of the editor and of the shortcuts dialog reports no violation", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const bar = page.getByRole("banner").getByRole("combobox", {
			name: "Language",
		});
		await bar.click();
		await page.getByRole("option", { name: "Português (Brasil)" }).click();
		await expect(
			page.getByRole("link", { name: "Voltar aos projetos" }),
		).toBeVisible();

		await page.locator("body").press("?");
		const dialog = page.getByRole("dialog", { name: "Atalhos de teclado" });
		await dialog.getByRole("combobox", { name: "Idioma" }).click();
		await page.getByRole("option", { name: "English" }).click();
		// The editor reloads in the chosen language; the option was clickable, so
		// the popup is above the dialog.
		await expect(
			page.getByRole("link", { name: "Back to projects" }),
		).toBeVisible();
	});
});
