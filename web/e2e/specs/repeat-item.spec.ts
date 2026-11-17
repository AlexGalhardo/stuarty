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

// The Repeat dialog of the item menu (`item.repeat`). No other spec sends it.
test.describe("repeat item", () => {
	test("confirming the dialog sends one item.repeat", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "1s",
		});
		const itemId = seeded.itemIds[0] ?? "";
		await page.goto(`/editor/${seeded.id}`);
		await page.locator("[data-item-id]").click({ button: "right" });
		const edits = recordEdits(page);
		await page.getByRole("menuitem", { name: "Repeat…" }).click();
		const dialog = page.getByRole("dialog");
		await expect(dialog).toBeVisible();
		await dialog.getByLabel("Copies", { exact: true }).fill("2");
		await dialog.getByRole("button", { name: "Repeat", exact: true }).click();
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toEqual([
			{
				op: "item.repeat",
				args: { item_id: itemId, copies: 2, ripple: "none" },
			},
		]);
	});
});
