import { expect, seedProject, test } from "./helpers";

// VV-178 (round L2): a required boolean left unchecked in a palette form is sent
// as false, not left out (which the core answers with E_VALIDATION).
test.describe("palette required boolean", () => {
	test("item.set_enabled runs with the checkbox untouched", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const itemId = seeded.itemIds[0];
		if (!itemId) throw new Error("no item seeded");
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await expect(
			page.getByRole("button", { name: /^Commands \(Ctrl\+K\)/ }),
		).toBeEnabled();

		await page.keyboard.press("Control+k");
		await page
			.getByRole("combobox", { name: "Search commands" })
			.fill("item.set_enabled");
		await page.locator('[data-command="item.set_enabled"]').click();
		const form = page.locator('[data-form="item.set_enabled"]');
		await expect(form).toBeVisible();
		await form.locator('[data-path="args.item_id"]').fill(itemId);
		await expect(form.locator('[data-path="args.enabled"]')).not.toBeChecked();
		await page.getByRole("button", { name: "Run", exact: true }).click();

		await expect(page.locator(`[data-item-id="${itemId}"]`)).toHaveAttribute(
			"data-disabled",
			"",
		);
	});
});
