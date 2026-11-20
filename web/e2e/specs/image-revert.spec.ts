import type { Page } from "@playwright/test";

import {
	editImage,
	expect,
	imageState,
	layerByName,
	newImage,
	test,
} from "./image-support";

// Epic 50: a creator names the version they have now, lets an agent rework the image, reads what changed, goes
// back to the named version in one step and takes that back, as they can with a video.
test.describe("image revert, named versions and the diff", () => {
	test.describe.configure({ timeout: 120_000 });

	const row = (page: Page, name: string) =>
		page.getByRole("treeitem", { name: new RegExp(`^${name},`) });

	test("name a version, let an agent work, read the changes, go back and undo the revert", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Revert spec" });
		await editImage(call, id, [
			{
				op: "layer.add_text",
				args: { text: "HOW IT WORKS", name: "Title" },
			},
		]);
		await page.goto(`/edit-image/${id}`);
		await expect(row(page, "Title")).toBeVisible();

		// the person names the version they have now
		await page.getByTestId("image-checkpoint-name").fill("Good title");
		await page.getByTestId("image-checkpoint-add").click();
		const named = page.locator('[data-checkpoint="Good title"]');
		await expect(named).toBeVisible();

		// an agent reworks the image: a new layer, and the title changed
		await editImage(call, id, [
			{
				op: "layer.add_shape",
				args: { shape: "ellipse", w: 200, h: 200, name: "Agent circle" },
			},
		]);
		await expect(row(page, "Agent circle")).toBeVisible();

		// the agent first edit of the day left an automatic version at the revision it started from
		await expect(
			page.locator("[data-checkpoint^='auto local:agent']"),
		).toBeVisible();

		// the person reads what changed since the named version, in words
		await named.getByTestId("image-checkpoint-revert").waitFor();
		const since = page
			.getByTestId("image-history-entry")
			.filter({ hasText: "revision 2" })
			.getByTestId("image-history-compare");
		await since.click();
		await expect(page.getByTestId("image-diff-lines")).toContainText(
			"added shape",
		);
		await expect(page.getByTestId("image-diff-lines")).toContainText(
			"Agent circle",
		);

		// "revert to here" shows the dry run first, and writes nothing until it is confirmed
		const before = (await imageState(call, id)).revision;
		await named.getByTestId("image-checkpoint-revert").click();
		const preview = page.getByTestId("image-revert-preview");
		await expect(preview).toContainText("removed shape");
		expect((await imageState(call, id)).revision).toBe(before);

		await page.getByTestId("image-revert-confirm").click();
		await expect(row(page, "Agent circle")).toHaveCount(0);
		await expect(row(page, "Title")).toBeVisible();

		const after = await imageState(call, id);
		expect(after.revision).toBe(before + 1);
		expect(layerByName(after.layers, "Agent circle")).toBeUndefined();

		// the revert is a step of the person: undo takes it back, and the agent circle returns
		await page.getByTestId("image-undo").click();
		await expect(row(page, "Agent circle")).toBeVisible();
		expect(
			layerByName((await imageState(call, id)).layers, "Agent circle"),
		).toBeDefined();
	});

	test("a version name is text and never markup, and a revert to the current state says so", async ({
		page,
		request,
	}) => {
		const { id } = await newImage(request, { name: "Hostile name spec" });
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByTestId("image-add-text")).toBeEnabled();

		await page
			.getByTestId("image-checkpoint-name")
			.fill("<img src=x onerror=alert(1)>");
		await page.getByTestId("image-checkpoint-add").click();

		const item = page.locator("[data-checkpoint]").first();
		await expect(item).toContainText("<img src=x onerror=alert(1)>");
		await expect(page.locator("img[src='x']")).toHaveCount(0);

		// the version points at the revision on screen, so going back to it changes nothing
		await item.getByTestId("image-checkpoint-revert").click();
		await expect(page.getByText(/already looks like this/)).toBeVisible();
		await expect(page.getByTestId("image-revert-confirm")).toBeDisabled();
	});
});
