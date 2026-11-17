import {
	audioClip,
	caller,
	expect,
	seedProject,
	test,
	testClip,
} from "./helpers";

// VV-159: the text label of an item is part of its hit area, so a click on the
// name selects the item like a click on its body.
test.describe("timeline-label-click", () => {
	test("clicking the label of a clip selects it", async ({ page, request }) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(2);
		const selected = page.locator('[data-item-id][aria-selected="true"]');
		await expect(selected).toHaveCount(0);

		const label = items.nth(1).locator("span[dir=auto]").first();
		await expect(label).toBeVisible();
		await label.click();
		await expect(selected).toHaveCount(1);
		await expect(items.nth(1)).toHaveAttribute("aria-selected", "true");
	});

	test("clicking the label of a long clip, off its first pixels, selects it", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "8s",
		});
		await page.goto(`/editor/${seeded.id}`);
		const item = page.locator("[data-item-id]");
		await expect(item).toHaveCount(1);
		await item.locator("span[dir=auto]").first().click();
		await expect(item).toHaveAttribute("aria-selected", "true");
	});

	test("clicking the label of an audio, text and shape item selects it", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			files: [testClip(), audioClip()],
			tracks: 1,
			audioTracks: 1,
			perTrack: 0,
		});
		const call = await caller(request);
		const head = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		const visual = seeded.trackIds[0];
		await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "audio.add",
					args: {
						track_id: seeded.audioTrackIds[0],
						asset_id: seeded.assetIds[1],
						start: "0s",
					},
				},
				{
					op: "text.add",
					args: { track_id: visual, start: "0s", duration: "4s", text: "Hi" },
				},
			],
		});
		await page.goto(`/editor/${seeded.id}`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(2);
		for (const index of [0, 1]) {
			await page.keyboard.press("Escape");
			const item = items.nth(index);
			await item.locator("span[dir=auto]").first().click();
			await expect(item).toHaveAttribute("aria-selected", "true");
		}
	});
});
