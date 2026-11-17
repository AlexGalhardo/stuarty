import AxeBuilder from "@axe-core/playwright";
import { expect, seedProject, test } from "./helpers";

// VV-161: each animatable Transform row has a labelled keyframe toggle (SDD-012
// section 6.5 item 4), one keyframe.set or keyframe.remove at the playhead.
test.describe("transform-keyframe-toggle", () => {
	test("the Transform rows toggle a keyframe at the playhead, and the panel passes axe", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await page.goto(`/editor/${seeded.id}`);
		await page.locator(`[data-item-id="${itemId}"]`).click();

		for (const name of ["X", "Y", "Scale", "Rotation", "Opacity"]) {
			await expect(
				page.getByRole("button", {
					name: `Add a keyframe for ${name} at the playhead`,
				}),
			).toBeVisible();
		}

		const { violations } = await new AxeBuilder({ page })
			.include('[aria-label="Inspector"]')
			.withTags(["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"])
			.analyze();
		expect(
			violations.filter(
				(v) => v.impact === "serious" || v.impact === "critical",
			),
		).toEqual([]);

		const add = page.getByRole("button", {
			name: "Add a keyframe for Opacity at the playhead",
		});
		await expect(add).toHaveAttribute("aria-pressed", "false");
		await add.click();
		const remove = page.getByRole("button", {
			name: "Remove the keyframe of Opacity at the playhead",
		});
		await expect(remove).toHaveAttribute("aria-pressed", "true");
		await expect(page.locator("[data-keyframe-row]")).toHaveCount(1);

		await remove.click();
		await expect(add).toHaveAttribute("aria-pressed", "false");
		await expect(page.locator("[data-keyframe-row]")).toHaveCount(0);
	});
});
