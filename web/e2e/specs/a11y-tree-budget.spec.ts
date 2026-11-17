import { expect, seedProject, test } from "./helpers";

// VV-191: the accessibility tree of the editor is what a small agent reads
// after every action, so its size has a budget. Measured on a project of
// 120 seconds (3 video tracks of 120 one-second clips, 1 audio track, 6
// assets) at 1280x720: before the change the tree had 430 lines (21,070
// characters) with nothing selected and 617 (25,896) with one clip selected;
// after it, 191 (6,612) and 412 (12,432). The limits keep about 30% of margin;
// a change that pushes the tree past them has to say why.
const LIMITS = {
	none: { lines: 250, chars: 8500 },
	selected: { lines: 540, chars: 16000 },
	// Clips listed per row, whatever the number of clips mounted.
	cells: 4 * 6 + 2,
};

test.describe("accessibility tree budget", () => {
	test.use({ viewport: { width: 1280, height: 720 } });

	test("the editor tree of a 2-minute project stays small", async ({
		page,
		request,
	}) => {
		test.setTimeout(180_000);
		const seeded = await seedProject(request, {
			tracks: 3,
			perTrack: 120,
			audioTracks: 1,
			assets: 6,
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]").first()).toBeVisible();
		await expect(page.locator("[data-item-id]")).not.toHaveCount(0);

		const none = await page.locator("body").ariaSnapshot();
		expect(none.split("\n").length).toBeLessThanOrEqual(LIMITS.none.lines);
		expect(none.length).toBeLessThanOrEqual(LIMITS.none.chars);
		const listed = none.split("\n").filter((l) => l.includes("gridcell"));
		expect(listed.length).toBeLessThanOrEqual(LIMITS.cells);

		// The clips stay on the screen and in the keyboard order.
		expect(await page.locator("[data-item-id]").count()).toBeGreaterThan(
			LIMITS.cells,
		);
		// The row says what it holds.
		expect(none).toMatch(/row "Video 1, 120 items, \d+ listed/);

		await page.locator("[data-item-id]").first().click();
		await expect(page.getByRole("button", { name: "Transform" })).toBeVisible();
		const one = await page.locator("body").ariaSnapshot();
		expect(one.split("\n").length).toBeLessThanOrEqual(LIMITS.selected.lines);
		expect(one.length).toBeLessThanOrEqual(LIMITS.selected.chars);
	});
});
