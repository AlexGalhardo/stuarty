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

// The mask row's keyframe toggle. Transform rows and the colour keyframe button
// already have specs; this one does not.
test.describe("mask keyframe", () => {
	test("the Mask X toggle sends keyframe.set at the playhead", async ({
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
		await page.getByLabel("Mask shape").selectOption("rect");
		const add = page.getByRole("button", {
			name: "Add a keyframe for Mask X at the playhead",
		});
		await expect(add).toBeEnabled();
		const edits = recordEdits(page);
		await add.click();
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toEqual([
			{
				op: "keyframe.set",
				args: {
					item_id: itemId,
					property: "mask_x",
					at: "0f",
					value: 0.1,
				},
			},
		]);
	});
});
