import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";
import { openInspectorTab } from "./sections";

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

// TASK-0254, SDD-012 sections 12.1 and 18.7 (NEW-36): the Colour section sends
// the HSL band, the wheel and the curve the person edits as one clip.set_color,
// and Reset sends the neutral value of what the clip stores.
test.describe("advanced colour", () => {
	test("HSL, wheels and curves send clip.set_color and Reset sends the neutral value", async ({
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

		await openInspectorTab(page, "Adjust");
		await page
			.getByRole("button", { name: "HSL by colour", exact: true })
			.click();
		await page.getByLabel("Colour range").selectOption("green");
		const hue = page.getByLabel("Hue shift", { exact: true });
		await hue.focus();
		await hue.press("End");
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toEqual([
			{
				op: "clip.set_color",
				args: {
					item_id: itemId,
					hsl: [{ range: "green", hue: 1, saturation: 0, lightness: 0 }],
				},
			},
		]);
		const resetHsl = page.getByRole("button", { name: "Reset HSL by colour" });
		await expect(resetHsl).toBeEnabled();
		await resetHsl.click();
		await expect.poll(() => edits.length).toBe(2);
		expect(edits[1]?.operations[0]?.args).toEqual({
			item_id: itemId,
			hsl: [{ range: "green", hue: 0, saturation: 0, lightness: 0 }],
		});

		await page
			.getByRole("button", { name: "Colour wheels", exact: true })
			.click();
		const midtones = page.getByRole("group", { name: "Midtones" });
		const amount = midtones.getByLabel("Amount", { exact: true });
		await amount.focus();
		await amount.press("End");
		await expect.poll(() => edits.length).toBe(3);
		expect(edits[2]?.operations[0]?.args).toEqual({
			item_id: itemId,
			wheels: { midtones: { hue: 0, amount: 1, luma: 0 } },
		});

		await page.getByRole("button", { name: "Curves", exact: true }).click();
		const first = page.getByRole("slider", { name: "Master curve, point 1" });
		await first.focus();
		await first.press("ArrowUp");
		await expect.poll(() => edits.length).toBe(4);
		expect(edits[3]?.operations[0]?.args).toEqual({
			item_id: itemId,
			curves: {
				master: [
					{ x: 0, y: 0.01 },
					{ x: 1, y: 1 },
				],
			},
		});
		await expect(
			page.getByRole("slider", { name: /^Master curve, point/ }),
		).toHaveCount(2);
		await page.getByRole("button", { name: "Add point" }).click();
		await expect.poll(() => edits.length).toBe(5);
		expect(edits[4]?.operations[0]?.args).toEqual({
			item_id: itemId,
			curves: {
				master: [
					{ x: 0, y: 0.01 },
					{ x: 0.5, y: 0.505 },
					{ x: 1, y: 1 },
				],
			},
		});
		await expect(
			page.getByRole("slider", { name: /^Master curve, point/ }),
		).toHaveCount(3);
		await page.getByRole("button", { name: "Reset Curves" }).click();
		await expect.poll(() => edits.length).toBe(6);
		expect(edits[5]?.operations[0]?.args).toEqual({
			item_id: itemId,
			curves: { master: [] },
		});
	});

	test("a colour keyframe button animates the property and its slider then edits the keyframe", async ({
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

		await openInspectorTab(page, "Adjust");
		await page
			.getByRole("button", {
				name: "Add a keyframe for Brightness at the playhead",
			})
			.click();
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toEqual([
			{
				op: "keyframe.set",
				args: { item_id: itemId, property: "brightness", at: "0f", value: 0 },
			},
		]);
		const remove = page.getByRole("button", {
			name: "Remove the keyframe of Brightness at the playhead",
		});
		await expect(remove).toHaveAttribute("aria-pressed", "true");

		const slider = page.getByLabel("Brightness", { exact: true });
		await slider.focus();
		await slider.press("End");
		await expect.poll(() => edits.length).toBe(2);
		expect(edits[1]?.operations[0]).toMatchObject({
			op: "keyframe.set",
			args: { item_id: itemId, property: "brightness", at: "0f", value: 1 },
		});

		await remove.click();
		await expect.poll(() => edits.length).toBe(3);
		expect(edits[2]?.operations).toEqual([
			{
				op: "keyframe.remove",
				args: { item_id: itemId, property: "brightness", at: "0f" },
			},
		]);
	});

	// VV-185: the core drops a neutral wheel, so the angle picked first is kept
	// by the panel and sent with the first tint.
	test("the hue of a neutral wheel is kept and sent with the first tint", async ({
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
		await openInspectorTab(page, "Adjust");
		await page
			.getByRole("button", { name: "Colour wheels", exact: true })
			.click();
		const edits = recordEdits(page);
		const shadows = page.getByRole("group", { name: "Shadows" });
		const hue = shadows.getByLabel("Hue (degrees)", { exact: true });
		await hue.focus();
		await hue.press("End");
		await expect(hue).toHaveValue("360");
		expect(edits).toHaveLength(0);

		const amount = shadows.getByLabel("Amount", { exact: true });
		await amount.focus();
		await amount.press("End");
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations[0]?.args).toEqual({
			item_id: itemId,
			wheels: { shadows: { hue: 360, amount: 1, luma: 0 } },
		});
		await expect(hue).toHaveValue("360");
	});
});
