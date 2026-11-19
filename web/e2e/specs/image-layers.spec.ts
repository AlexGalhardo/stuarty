import type { Page } from "@playwright/test";

import {
	editImage,
	expect,
	type ImageLayer,
	imageState,
	layerByName,
	newImage,
	test,
} from "./image-support";

// Epic 28 task 4: the layer panel is a tree; reorder, group, rename, hide and lock by pointer and by keyboard.

async function seed(request: Parameters<typeof newImage>[0]) {
	const { id, call } = await newImage(request, { name: "Layers spec" });
	await editImage(call, id, [
		{ op: "layer.add_fill", args: { color: "#203040", name: "Back" } },
		{
			op: "layer.add_shape",
			args: { shape: "rect", w: 100, h: 60, x: 50, y: 50, name: "Middle" },
		},
		{
			op: "layer.add_shape",
			args: { shape: "ellipse", w: 80, h: 80, x: 400, y: 100, name: "Top" },
		},
	]);
	const names = async (): Promise<string[]> =>
		(await imageState(call, id)).layers.map((l) => l.name);
	const layer = async (name: string): Promise<ImageLayer> => {
		const found = layerByName((await imageState(call, id)).layers, name);
		if (!found) throw new Error(`no layer ${name}`);
		return found;
	};
	return { id, call, names, layer };
}

const row = (page: Page, name: string) =>
	page.getByRole("treeitem", { name: new RegExp(`^${name},`) });

test.describe("image layers", () => {
	test.describe.configure({ timeout: 90_000 });
	test("is a tree, top layer first, with the roles and counts of its items", async ({
		page,
		request,
	}) => {
		const { id } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		const tree = page.getByRole("tree", { name: "Layers, top first" });
		await expect(tree).toBeVisible();
		const items = tree.getByRole("treeitem");
		await expect(items).toHaveCount(3);
		await expect(items.nth(0)).toHaveAccessibleName(/^Top, shape/);
		await expect(items.nth(2)).toHaveAccessibleName(/^Back, fill/);
		await expect(items.nth(0)).toHaveAttribute("aria-level", "1");
		await expect(items.nth(0)).toHaveAttribute("aria-posinset", "1");
		await expect(items.nth(0)).toHaveAttribute("aria-setsize", "3");
	});

	test("a click selects the layer on the canvas, and the selection on the canvas selects the row", async ({
		page,
		request,
	}) => {
		const { id, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByTestId("image-frame")).toBeVisible({
			timeout: 30_000,
		});
		const middle = await layer("Middle");
		await row(page, "Middle").click();
		await expect(row(page, "Middle")).toHaveAttribute("aria-selected", "true");
		await expect(page.getByTestId("image-selection")).toHaveAttribute(
			"data-layer",
			middle.id,
		);

		const top = await layer("Top");
		const box = await page.getByTestId("image-picture").boundingBox();
		const zoom = Number(
			await page.getByTestId("image-stage").getAttribute("data-zoom"),
		);
		if (!box) throw new Error("no picture");
		await page.mouse.click(
			box.x + (top.x + 20) * zoom,
			box.y + (top.y + 20) * zoom,
		);
		await expect(row(page, "Top")).toHaveAttribute("aria-selected", "true");
		await expect(row(page, "Middle")).toHaveAttribute("aria-selected", "false");
	});

	test("Alt with Up and Down moves a layer one place among its siblings", async ({
		page,
		request,
	}) => {
		const { id, names } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Middle").click();
		await page.keyboard.press("Alt+ArrowUp");
		await expect.poll(names).toEqual(["Back", "Top", "Middle"]);
		await page.keyboard.press("Alt+ArrowDown");
		await page.keyboard.press("Alt+ArrowDown");
		await expect.poll(names).toEqual(["Middle", "Back", "Top"]);
	});

	test("drag and drop reorders: a row dropped above another goes above it", async ({
		page,
		request,
	}) => {
		const { id, names } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		const back = row(page, "Back");
		const top = row(page, "Top");
		const box = await top.boundingBox();
		if (!box) throw new Error("no row");
		await back.dragTo(top, { targetPosition: { x: 40, y: 3 } });
		await expect.poll(names).toEqual(["Middle", "Top", "Back"]);
	});

	test("Ctrl+G groups the selection, a group opens and closes, and Alt+Left takes a layer out", async ({
		page,
		request,
	}) => {
		const { id, call, names } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Middle").click();
		await row(page, "Top").click({ modifiers: ["Control"] });
		await page.keyboard.press("Control+g");
		await expect.poll(names).toEqual(["Back", "Group"]);

		const group = row(page, "Group");
		await expect(group).toHaveAttribute("aria-expanded", "true");
		await expect(page.getByRole("treeitem")).toHaveCount(4);
		await page.keyboard.press("ArrowLeft");
		await expect(group).toHaveAttribute("aria-expanded", "false");
		await expect(page.getByRole("treeitem")).toHaveCount(2);
		await page.keyboard.press("ArrowRight");
		await expect(page.getByRole("treeitem")).toHaveCount(4);

		const child = row(page, "Top");
		await expect(child).toHaveAttribute("aria-level", "2");
		await child.click();
		await page.keyboard.press("Alt+ArrowLeft");
		await expect
			.poll(async () => (await imageState(call, id)).layers.map((l) => l.name))
			.toEqual(["Back", "Group", "Top"]);

		await row(page, "Group").click();
		await page.keyboard.press("Control+Shift+g");
		await expect.poll(names).toEqual(["Back", "Middle", "Top"]);
	});

	test("F2 renames and Enter keeps the name", async ({ page, request }) => {
		const { id, names } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Middle").click();
		await page.keyboard.press("F2");
		const field = page.getByRole("textbox", { name: "Layer name" });
		await field.fill("Banner");
		await field.press("Enter");
		await expect.poll(names).toContain("Banner");
		await expect(row(page, "Banner")).toBeFocused();
	});

	test("H hides and L locks the focused layer; the icons do the same by pointer", async ({
		page,
		request,
	}) => {
		const { id, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Top").click();
		await page.keyboard.press("h");
		await expect.poll(async () => (await layer("Top")).visible).toBe(false);
		await expect(row(page, "Top")).toHaveAccessibleName(/hidden/);
		await row(page, "Top").getByTestId("image-layer-visible").click();
		await expect.poll(async () => (await layer("Top")).visible).toBe(true);

		await row(page, "Top").getByTestId("image-layer-lock").click();
		await expect.poll(async () => (await layer("Top")).locked).toBe(true);
		await expect(row(page, "Top")).toHaveAccessibleName(/locked/);
		// a locked layer is selected but not dragged and not removed
		await page.keyboard.press("l");
		await expect.poll(async () => (await layer("Top")).locked).toBeFalsy();
	});

	test("Delete removes the focused layer and the empty state shows when none is left", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Empty spec" });
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#000000", name: "Only" } },
		]);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Only").click();
		await page.keyboard.press("Delete");
		await expect(page.getByTestId("image-layers-empty")).toBeVisible();
		await expect(page.getByText("The image has no layers")).toBeVisible();
	});

	test("the toolbar groups, duplicates, reorders and removes the selection", async ({
		page,
		request,
	}) => {
		const { id, names } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Middle").click();
		await page.getByTestId("image-forward").click();
		await expect.poll(names).toEqual(["Back", "Top", "Middle"]);
		await page.getByTestId("image-back").click();
		await expect.poll(names).toEqual(["Back", "Middle", "Top"]);
		await page.getByTestId("image-duplicate").click();
		await expect.poll(async () => (await names()).length).toBe(4);
		await page.getByTestId("image-remove").click();
		await expect.poll(async () => (await names()).length).toBe(3);
	});
});
