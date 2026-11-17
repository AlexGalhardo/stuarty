import { expect, seedProject, test } from "./helpers";

// TASK-0287 (SDD-012 section 6.4): the Transitions and Filters tabs. The cards
// are the lists of the registry; applying with nothing selected is disabled
// with the reason in aria-describedby; each card sends one command.
test.describe("library transitions and filters tabs", () => {
	test.use({ viewport: { width: 1600, height: 900 } });

	test("a transition card applies to the selected clip", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("tab", { name: "Transitions" }).click();
		const panel = page.getByRole("region", { name: "Transitions" });
		const fade = panel.getByRole("button", { name: "Fade", exact: true });
		await expect(fade).toHaveAttribute("aria-disabled", "true");
		await expect(fade).toHaveAccessibleDescription("Select a clip first.");

		await page.locator(`[data-item-id="${seeded.itemIds[1]}"]`).click();
		await panel.getByLabel("Out").check();
		const edit = page.waitForRequest(
			(r) => r.method() === "POST" && r.url().endsWith("/edit"),
		);
		await fade.click();
		const body = (await edit).postDataJSON() as {
			operations: { op: string; args: Record<string, unknown> }[];
		};
		expect(body.operations).toEqual([
			{
				op: "item.set_transition",
				args: {
					item_id: seeded.itemIds[1],
					edge: "out",
					kind: "fade",
				},
			},
		]);
	});

	test("a filter card applies and the amount follows the stored filter", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("tab", { name: "Filters" }).click();
		const panel = page.getByRole("region", { name: "Filters" });
		const sepia = panel.getByRole("button", { name: "Sepia" });
		await expect(sepia).toHaveAttribute("aria-disabled", "true");
		const amount = panel.getByRole("slider");
		await expect(amount).toBeDisabled();

		await page.locator(`[data-item-id="${seeded.itemIds[0]}"]`).click();
		const edit = page.waitForRequest(
			(r) => r.method() === "POST" && r.url().endsWith("/edit"),
		);
		await sepia.click();
		const body = (await edit).postDataJSON() as {
			operations: { op: string; args: Record<string, unknown> }[];
		};
		expect(body.operations[0]).toEqual({
			op: "clip.set_filter",
			args: { item_id: seeded.itemIds[0], filter: "sepia" },
		});
		await expect(sepia).toHaveAttribute("aria-pressed", "true");
		await expect(amount).toBeEnabled();
	});
});
