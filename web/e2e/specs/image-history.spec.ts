import type { Page } from "@playwright/test";

import {
	editImage,
	expect,
	imageState,
	layerByName,
	newImage,
	test,
} from "./image-support";

// Epic 28 task 8 and acceptance criterion 2: a mixed session. An agent's command shows on the canvas without a
// reload, the history names who did what, and the person undoes their own last step and not the agent's.
test.describe("image history and other actors", () => {
	test.describe.configure({ timeout: 90_000 });

	const row = (page: Page, name: string) =>
		page.getByRole("treeitem", { name: new RegExp(`^${name},`) });

	test("a command of an agent appears without reloading, outlined and announced, and the history names it", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Mixed spec" });
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#203040", name: "Back" } },
		]);
		await page.goto(`/edit-image/${id}`);
		await expect(row(page, "Back")).toBeVisible();
		const frame = page.getByTestId("image-frame");
		await expect(frame).toBeVisible();
		const before = Number(await frame.getAttribute("data-revision"));

		await editImage(call, id, [
			{
				op: "layer.add_shape",
				args: { shape: "ellipse", w: 200, h: 200, name: "Agent circle" },
			},
		]);

		// no reload: the tree, the picture and the history follow by themselves
		await expect(row(page, "Agent circle")).toBeVisible();
		await expect(frame).toHaveAttribute("data-revision", String(before + 1));
		await expect(page.getByTestId("image-agent-notice")).toContainText(
			"local:agent",
		);
		await expect(page.getByTestId("image-changed")).toHaveCount(1);
		const entry = page
			.getByTestId("image-history-entry")
			.filter({ hasText: "layer.add_shape" });
		await expect(entry).toHaveAttribute("data-actor", "local:agent");
		await expect(entry.getByTestId("image-history-actor")).toHaveText(
			"local:agent",
		);
	});

	test("the person undoes their own last step and the agent's step stays", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Own undo spec" });
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByTestId("image-add-text")).toBeEnabled();

		// the person adds a text; the agent adds a shape afterwards
		await page.getByTestId("image-add-text").click();
		await expect(row(page, "Text")).toBeVisible();
		await editImage(call, id, [
			{
				op: "layer.add_shape",
				args: { shape: "rect", w: 100, h: 100, name: "Agent box" },
			},
		]);
		await expect(row(page, "Agent box")).toBeVisible();

		await page.getByTestId("image-undo").click();
		await expect
			.poll(async () => {
				const names = (await imageState(call, id)).layers.map((l) => l.name);
				return names.join(",");
			})
			.toBe("Agent box");
		await expect(row(page, "Text")).toHaveCount(0);
		await expect(row(page, "Agent box")).toBeVisible();
		// the log says so, in words: the step is undone, by whom
		const entries = page.getByTestId("image-history-entry");
		await expect(entries.first()).toContainText("undid revision");
		await expect(page.getByText(/Undone by revision/)).toBeVisible();

		// redo brings the person's step back
		await page.getByTestId("image-redo").click();
		await expect
			.poll(async () => (await imageState(call, id)).layers.length)
			.toBe(2);
	});

	test("an object the agent changed since is left alone by the person's undo, and the log says so", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Skipped spec" });
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByTestId("image-add-rect")).toBeEnabled();
		await page.getByTestId("image-add-rect").click();
		await expect(row(page, "Shape")).toBeVisible();
		const made = layerByName((await imageState(call, id)).layers, "Shape");
		if (!made) throw new Error("no shape");

		// the agent changes that very layer, so the person's step is no longer theirs alone
		await editImage(call, id, [
			{ op: "layer.transform", args: { layer_id: made.id, x: 77 } },
		]);
		await expect
			.poll(
				async () =>
					(await layerByName((await imageState(call, id)).layers, "Shape"))?.x,
			)
			.toBe(77);
		await expect(page.getByTestId("image-agent-notice")).toContainText(
			"local:agent",
		);

		await page.getByTestId("image-undo").click();
		await expect(
			page.getByText(/left alone because another actor/),
		).toBeVisible();
		// the agent's change is still there
		expect(layerByName((await imageState(call, id)).layers, "Shape")?.x).toBe(
			77,
		);
	});

	test("Ctrl+Z and Ctrl+Shift+Z undo and redo from the canvas, but not inside a field", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Keys spec" });
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByTestId("image-add-fill")).toBeEnabled();
		await page.getByTestId("image-add-fill").click();
		await expect(row(page, "Fill")).toBeVisible();
		await page.getByTestId("image-stage").focus();
		await page.keyboard.press("Control+z");
		await expect
			.poll(async () => (await imageState(call, id)).layers.length)
			.toBe(0);
		await page.keyboard.press("Control+Shift+z");
		await expect
			.poll(async () => (await imageState(call, id)).layers.length)
			.toBe(1);

		// inside the name field Ctrl+Z is the field's own and the document stays
		await row(page, "Fill").click();
		const name = page.getByTestId("image-field-name");
		await name.fill("Typing");
		const revision = (await imageState(call, id)).revision;
		await name.press("Control+z");
		await page.getByTestId("image-stage").focus();
		expect((await imageState(call, id)).revision).toBe(revision);
	});
});
