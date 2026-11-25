import { expect, imageState, newImage, test } from "./image-support";

// Epic 99 group C: the command palette (Ctrl K) lists the operations the editor offers and runs the chosen one
// through the same call as its button.
test.describe("image editor command palette", () => {
	test.describe.configure({ timeout: 90_000 });

	test("Ctrl K opens a labelled dialog, the arrows and Enter run an operation, Escape closes", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Palette spec" });
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByTestId("image-frame")).toBeVisible();
		const before = (await imageState(call, id)).layers.length;

		await page.getByTestId("image-add-text").focus();
		await page.keyboard.press("Control+k");
		const dialog = page.getByRole("dialog", { name: "Commands" });
		await expect(dialog).toBeVisible();
		const input = page.getByRole("combobox", { name: "Search a command" });
		await expect(input).toBeFocused();
		await expect(page.getByRole("listbox", { name: "Commands" })).toBeVisible();

		// filter, then Enter runs the first available match
		await input.fill("rectangle");
		await expect(page.getByRole("option")).toHaveCount(1);
		await expect(page.getByRole("option", { selected: true })).toContainText(
			"rectangle",
		);
		await page.keyboard.press("Enter");
		await expect(dialog).toBeHidden();
		await expect
			.poll(async () => (await imageState(call, id)).layers.length)
			.toBe(before + 1);

		// the arrows move the active option; Escape closes without running
		await page.keyboard.press("Control+k");
		await expect(dialog).toBeVisible();
		await input.fill("add");
		const first = page.getByRole("option", { selected: true });
		const name = await first.textContent();
		await page.keyboard.press("ArrowDown");
		expect(
			await page.getByRole("option", { selected: true }).textContent(),
		).not.toBe(name);
		await page.keyboard.press("Escape");
		await expect(dialog).toBeHidden();
		expect((await imageState(call, id)).layers.length).toBe(before + 1);
	});

	test("Ctrl K inside another dialog stays there: no palette over the import dialog", async ({
		page,
		request,
	}) => {
		const { id } = await newImage(request, { name: "Palette over dialog" });
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByTestId("image-frame")).toBeVisible();
		await page.getByTestId("image-import-open").click();
		const importing = page.getByTestId("image-import-dialog");
		await expect(importing).toBeVisible();
		await importing.locator("input:not([type=file])").first().focus();
		await page.keyboard.press("Control+k");
		await expect(importing).toBeVisible();
		await expect(page.getByTestId("image-palette")).toHaveCount(0);
	});

	test("the top bar button opens it and a click on an option runs it; focus stays inside while open", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Palette click" });
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByTestId("image-frame")).toBeVisible();
		await page.getByTestId("image-palette-open").click();
		const dialog = page.getByRole("dialog", { name: "Commands" });
		await expect(dialog).toBeVisible();
		for (let i = 0; i < 6; i += 1) await page.keyboard.press("Tab");
		expect(
			await page.evaluate(
				() => document.activeElement?.closest('[role="dialog"]') !== null,
			),
		).toBe(true);
		await page.getByTestId("image-palette-add-ellipse").click();
		await expect(dialog).toBeHidden();
		await expect
			.poll(async () => (await imageState(call, id)).layers.length)
			.toBe(1);
	});
});
