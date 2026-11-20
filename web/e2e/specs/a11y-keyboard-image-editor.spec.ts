import { unnamedControls } from "../support/keyboard";
import { editImage, expect, imageState, newImage, test } from "./image-support";

// Epic 41 task 5 (SDD-012 section 18.10, item N and the keyboard model of the image canvas): the layers are a tree
// whose rows say their level and place, the selection is told in a polite region, and every control of the
// editor has a name and a role in the accessibility tree. The keyboard session itself is in a11y-image-editor.
test.describe("a11y-keyboard-image-editor", () => {
	async function seed(request: Parameters<typeof newImage>[0]) {
		const { id, call } = await newImage(request, { name: "Tree spec" });
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#203040", name: "Back" } },
			{
				op: "layer.add_shape",
				args: {
					shape: "rounded_rect",
					w: 300,
					h: 120,
					x: 100,
					y: 100,
					radius: 12,
					name: "Box",
				},
			},
			{
				op: "layer.add_text",
				args: {
					text: "HEADLINE",
					name: "Title",
					style: { size: 100, color: "#FFE600" },
				},
			},
		]);
		return { id, call };
	}

	test("the layers are a tree: every layer is a row with its level, set size and place, top layer first", async ({
		page,
		request,
	}) => {
		const { id, call } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		const rows = page.getByRole("treeitem");
		await expect(rows).toHaveCount(3);
		const names = (await imageState(call, id)).layers
			.map((layer) => layer.name)
			.reverse();
		for (const [index, name] of names.entries()) {
			const row = rows.nth(index);
			await expect(row).toHaveAccessibleName(new RegExp(`^${name},`));
			await expect(row).toHaveAttribute("aria-level", "1");
			await expect(row).toHaveAttribute("aria-setsize", "3");
			await expect(row).toHaveAttribute("aria-posinset", String(index + 1));
		}
		await expect(page.getByRole("tree")).toHaveAccessibleName(/.+/);
	});

	test("the canvas tells the selection and where a nudge put it", async ({
		page,
		request,
	}) => {
		const { id } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		const stage = page.getByTestId("image-stage");
		await stage.focus();
		await page.keyboard.press("PageUp");
		const status = page
			.getByRole("status")
			.filter({ hasText: /^Layer .+, at -?\d+, -?\d+, \d+ by \d+$/ });
		await expect(status.first()).toBeVisible();
		const before = (await status.first().textContent()) ?? "";
		await page.keyboard.press("Shift+ArrowRight");
		await expect(status.first()).not.toHaveText(before);
	});

	test("every control of the image editor has a name and a role in the accessibility tree", async ({
		page,
		request,
	}) => {
		const { id } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByRole("treeitem")).toHaveCount(3);
		await page.getByRole("treeitem").first().click();
		const tree = await page.locator("main").ariaSnapshot();
		expect(tree).toMatch(/treeitem "/);
		expect(tree).toMatch(/toolbar|button "/);
		expect(unnamedControls(tree)).toEqual([]);
	});
});
