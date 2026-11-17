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

// Shadow and Outline (SDD-012 section 12.1): each switch and each field is one
// clip.set_shadow or clip.set_outline. The preview does not draw those pixels.
test.describe("shadow and outline", () => {
	test("the shadow and outline controls send their commands", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const itemId = seeded.itemIds[0] ?? "";
		await page.goto(`/editor/${seeded.id}`);
		await page.locator("[data-item-id]").click();
		await expect(page.getByRole("heading", { name: /^Clip/ })).toBeVisible();
		const edits = recordEdits(page);

		await page.getByRole("checkbox", { name: "Shadow", exact: true }).check();
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toEqual([
			{ op: "clip.set_shadow", args: { item_id: itemId, shadow: {} } },
		]);

		const shadowColour = page.getByLabel("Shadow colour", { exact: true });
		await expect(shadowColour).toBeVisible();
		await shadowColour.fill("#FF8800");
		await shadowColour.press("Enter");
		await expect.poll(() => edits.length).toBe(2);
		expect(edits[1]?.operations).toEqual([
			{
				op: "clip.set_shadow",
				args: { item_id: itemId, shadow: { color: "#FF8800" } },
			},
		]);

		await page.getByRole("checkbox", { name: "Outline", exact: true }).check();
		await expect.poll(() => edits.length).toBe(3);
		expect(edits[2]?.operations).toEqual([
			{ op: "clip.set_outline", args: { item_id: itemId, outline: {} } },
		]);

		const width = page.getByLabel("Outline width", { exact: true });
		await expect(width).toBeVisible();
		// Width 4 is the stored default, and a draft equal to it sends nothing.
		await width.fill("6");
		await width.press("Enter");
		await expect.poll(() => edits.length).toBe(4);
		expect(edits[3]?.operations).toEqual([
			{
				op: "clip.set_outline",
				args: { item_id: itemId, outline: { width: 6 } },
			},
		]);
	});
});
