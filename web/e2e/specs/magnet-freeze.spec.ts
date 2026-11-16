import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

function recordEdits(page: Page): Posted[] {
	const edits: Posted[] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			edits.push(r.postDataJSON() as Posted);
		}
	});
	return edits;
}

// TASK-0254, SDD-012 section 18.7 (N-6) and section 8.2: the P key and the
// Magnet button send `track.update` with `magnet`, and Freeze frame sends
// `item.freeze_frame` at the playhead. The page sends the command and draws
// what the core answers.
test.describe("magnet-freeze", () => {
	test("P and the Magnet button turn the magnet of the track of the selected item on and off", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		await page.locator("[data-item-id]").first().click();
		const edits = recordEdits(page);
		const button = page.getByRole("button", { name: "Magnet", exact: true });
		await expect(button).toHaveAttribute("aria-pressed", "false");

		await page.keyboard.press("p");
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toEqual([
			{
				op: "track.update",
				args: { track_id: seeded.trackIds[0], magnet: true },
			},
		]);
		await expect(button).toHaveAttribute("aria-pressed", "true");

		await button.click();
		await expect.poll(() => edits.length).toBe(2);
		expect(edits[1]?.operations[0]?.args).toMatchObject({ magnet: false });
		await expect(button).toHaveAttribute("aria-pressed", "false");
	});

	test("Freeze frame sends one item.freeze_frame at the playhead and the still appears with the answer", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("button", { name: "Step forward one frame" }).click();
		await page.getByRole("button", { name: "Step forward one frame" }).click();
		await page.locator("[data-item-id]").click();
		const edits = recordEdits(page);

		await page.getByRole("button", { name: "Freeze frame" }).click();
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toEqual([
			{
				op: "item.freeze_frame",
				args: { item_id: seeded.itemIds[0], at: "2f" },
			},
		]);
		await expect(page.locator("[data-item-id]")).toHaveCount(3);
	});
});
