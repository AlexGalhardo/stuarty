import { caller, expect, makeProject, test } from "./helpers";

// VV-433: the dialog that edits a project puts the focus in its first field, and one Escape closes it. The focus
// used to land on the help button before the name, whose tooltip took the first Escape.
test.describe("project edit dialog", () => {
	test("opens on the name field and closes with one Escape", async ({
		page,
		request,
	}) => {
		const call = await caller(request, "e2e-edit-dialog");
		const made = await makeProject(call, "Edit focus project");
		await page.goto(`/projects/${made.project_id as string}`);
		await page.getByRole("button", { name: "Edit the project" }).click();
		const dialog = page.getByRole("dialog");
		await expect(dialog).toBeVisible();
		await expect(dialog.getByRole("textbox", { name: "Name" })).toBeFocused();
		await page.keyboard.press("Escape");
		await expect(dialog).toBeHidden();
	});
});
