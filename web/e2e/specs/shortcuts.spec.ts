import { expect, seedProject, test } from "./helpers";

// P3-UI-001 (SDD-012 section 18.2): the cheat sheet opens with ?, rebinding
// Mod+B to X makes X split (one batch) and Mod+B do nothing, and the binding
// survives a reload; a key another row holds is refused; reset brings Mod+B back.
test.describe("shortcuts", () => {
	test("rebinding Mod+B to X splits with X, keeps across a reload, conflicts are refused", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const batches: string[][] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				const body = r.postDataJSON() as { operations: { op: string }[] };
				batches.push(body.operations.map((o) => o.op));
			}
		});
		await page.goto(`/editor/${seeded.id}`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(1);

		// The cheat sheet is generated from the table and opens with ?.
		await page.locator("body").press("?");
		const dialog = page.getByRole("dialog", { name: "Keyboard shortcuts" });
		await expect(dialog).toBeVisible();
		await expect(dialog.locator('[data-shortcut-row="split"]')).toContainText(
			/(Ctrl|Cmd)\+B/,
		);
		await expect(dialog.locator('[data-shortcut-row="undo"]')).toContainText(
			/(Ctrl|Cmd)\+Z/,
		);

		// A key that another row holds is refused and names that row.
		const split = dialog.locator('[data-shortcut-key="split"]');
		await split.click();
		await split.press("c");
		await expect(dialog.getByRole("alert")).toContainText("Add caption");
		await expect(split).toContainText(/(Ctrl|Cmd)\+B/);

		// Mod+B to X.
		await split.click();
		await split.press("x");
		await expect(split).toContainText("X");
		await page.keyboard.press("Escape");
		await expect(dialog).toBeHidden();

		await page.getByRole("slider", { name: "Playhead" }).focus();
		await page.keyboard.press("PageUp");
		await items.first().focus();
		await page.keyboard.press("Enter");

		await page.keyboard.press("Control+b");
		await page.waitForTimeout(500);
		await expect(items).toHaveCount(1);
		expect(batches).toHaveLength(0);

		await page.keyboard.press("x");
		await expect(items).toHaveCount(2);
		expect(batches).toHaveLength(1);

		// The binding survives a reload.
		await page.reload();
		await expect(items).toHaveCount(2);
		await page.locator("body").press("?");
		await expect(
			page
				.getByRole("dialog", { name: "Keyboard shortcuts" })
				.locator('[data-shortcut-key="split"]'),
		).toContainText("X");

		// Reset to defaults.
		await page.getByRole("button", { name: "Reset to defaults" }).click();
		await expect(
			page
				.getByRole("dialog", { name: "Keyboard shortcuts" })
				.locator('[data-shortcut-key="split"]'),
		).toContainText(/(Ctrl|Cmd)\+B/);
	});
});
