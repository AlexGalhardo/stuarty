import { expect, seedProject, test } from "./helpers";

// VV-160: "Add shape" is a menu with menu and menuitem roles, reachable by the
// keyboard, and the accessible name "Rectangle" resolves to its item even while
// the mask shape select of the inspector (which also lists "Rectangle") is on
// screen.
test.describe("shape-menu", () => {
	test("the menu has the menu role and a keyboard path, and Rectangle is one menu item", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await page.locator("[data-item-id]").first().click();
		const trigger = page.getByRole("button", { name: "Add shape" });
		await expect(trigger).toBeEnabled();

		await trigger.focus();
		await page.keyboard.press("Enter");
		const menu = page.getByRole("menu");
		await expect(menu).toBeVisible();
		const rectangle = page.getByRole("menuitem", {
			name: "Rectangle",
			exact: true,
		});
		// One item, although the mask shape select of the inspector, a native
		// select, also has an option named Rectangle (an option is not a menu item).
		await expect(rectangle).toHaveCount(1);

		const posted: string[] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				posted.push(r.postData() ?? "");
			}
		});
		await page.keyboard.press("Enter");
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]).toContain('"shape":"rect"');
		await expect(menu).toBeHidden();
	});
});
