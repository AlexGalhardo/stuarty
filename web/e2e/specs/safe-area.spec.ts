import { expect, seedProject, test } from "./helpers";

interface Plan {
	width: number;
	height: number;
	safe_area: [number, number, number, number];
	layers: { item: string }[];
}

// P2-UI-015 (SDD-012 section 11.6): the safe-area guide is the rectangle the
// plan window names, drawn at the scale of the fit; a click on the canvas
// selects the topmost layer and the selection box follows the selected item.
test.describe("safe-area", () => {
	test("the guide rectangle equals the safe_area of the plan window at fit zoom", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const plans: Plan[] = [];
		page.on("response", async (response) => {
			if (!response.url().includes("/plan?") || !response.ok()) return;
			plans.push((await response.json()) as Plan);
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await expect.poll(() => plans.length).toBeGreaterThan(0);
		await expect(page.getByTestId("safe-area")).toHaveCount(0);

		const toggle = page.getByRole("button", { name: "Safe area" });
		await expect(toggle).toHaveAttribute("aria-pressed", "false");
		await toggle.click();
		await expect(toggle).toHaveAttribute("aria-pressed", "true");

		const plan = plans[0] as Plan;
		const canvas = await page.getByTestId("canvas-overlay").boundingBox();
		const guide = await page.getByTestId("safe-area").boundingBox();
		if (canvas === null || guide === null) throw new Error("not laid out");
		const scale = canvas.width / plan.width;
		const [x, y, w, h] = plan.safe_area;
		expect(Math.abs(guide.x - canvas.x - x * scale)).toBeLessThan(1);
		expect(Math.abs(guide.y - canvas.y - y * scale)).toBeLessThan(1);
		expect(Math.abs(guide.width - w * scale)).toBeLessThan(1);
		expect(Math.abs(guide.height - h * scale)).toBeLessThan(1);

		await toggle.click();
		await expect(page.getByTestId("safe-area")).toHaveCount(0);
	});

	test("a click on the canvas selects the topmost layer and draws its box; the box is gone outside the item", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const item = page.locator("[data-item-id]");
		await expect(item).toHaveCount(1);
		const id = seeded.itemIds[0] as string;
		await expect(page.getByTestId("selection-box")).toHaveCount(0);

		const canvas = page.getByTestId("canvas-overlay");
		await expect(async () => {
			await canvas.click();
			await expect(item).toHaveAttribute("aria-selected", "true", {
				timeout: 500,
			});
		}).toPass();
		await expect(page.getByTestId("selection-box")).toHaveAttribute(
			"data-item",
			id,
		);

		// At the end of the project the playhead is outside the item.
		await page.getByRole("slider", { name: "Playhead" }).focus();
		await page.keyboard.press("End");
		await expect(page.getByTestId("selection-box")).toHaveCount(0);
	});
});
