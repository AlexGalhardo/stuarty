import { expect, seedProject, test } from "./helpers";

// TASK-0242: the patterns of the research note capcut-gui (SDD-012 section 6.5).
test.describe("toolbar tooltips and toggles", () => {
	test("a tool names its shortcut in the tooltip", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await expect(page.getByRole("button", { name: "Split" })).toHaveAttribute(
			"title",
			/^Split \((Ctrl|Cmd)\+B\)$/,
		);
		await expect(page.getByRole("button", { name: "Undo" })).toHaveAttribute(
			"title",
			/^Undo \((Ctrl|Cmd)\+Z\)$/,
		);
		await expect(
			page.getByRole("button", { name: "Snap", exact: true }),
		).toHaveAttribute("title", "Snap (N)");
	});
});
