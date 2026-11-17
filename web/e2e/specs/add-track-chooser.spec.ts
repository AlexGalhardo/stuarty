import { expect, seedProject, test } from "./helpers";

// VV-092: "Add to timeline" has a track chooser: an existing track of the kind
// or a new one, and the start stays with the core (the end of the track).
test.describe("add to timeline track chooser", () => {
	test("adds to the chosen track and to a new track", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 2, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		const chosen = seeded.trackIds[0];
		if (chosen === undefined) throw new Error("no track seeded");

		await page.getByRole("button", { name: /Choose the track for/ }).click();
		await page.locator(`[role="menuitem"][data-track-id="${chosen}"]`).click();
		await expect(page.locator("[data-item-id]")).toHaveCount(3);
		const onChosen = await page.evaluate((id) => {
			const doc = window.__vv?.document as {
				items: Record<string, { track: string }>;
			};
			return Object.values(doc.items).filter((i) => i.track === id).length;
		}, chosen);
		expect(onChosen).toBe(2);

		await page.getByRole("button", { name: /Choose the track for/ }).click();
		await page.getByRole("menuitem", { name: "New track" }).click();
		await expect(page.locator("[data-item-id]")).toHaveCount(4);
		const tracks = await page.evaluate(() => {
			const doc = window.__vv?.document as
				| { tracks: Record<string, unknown> }
				| undefined;
			return Object.keys(doc?.tracks ?? {}).length;
		});
		expect(tracks).toBe(3);
	});
});
