import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

function recordEdits(page: Page): Posted[] {
	const posted: Posted[] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			posted.push(r.postDataJSON() as Posted);
		}
	});
	return posted;
}

async function storedKeyframes(
	page: Page,
	itemId: string,
): Promise<Record<string, { t: number; value: number; easing?: string }[]>> {
	return page.evaluate((id) => {
		const doc = window.__vv?.document as {
			items: Record<
				string,
				{
					keyframes?: Record<
						string,
						{ t: number; value: number; easing?: string }[]
					>;
				}
			>;
		};
		return doc.items[id]?.keyframes ?? {};
	}, itemId);
}

const LATER = 10;

// P2-UI-020 (SDD-012 sections 7.4 and 12.1): keyframes are added at the
// playhead, listed per property, removed and cleared; every action is one
// command and the browser works out no value between two keyframes.
test.describe("keyframes", () => {
	test("add at the playhead, move it and add another, remove, clear", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${itemId}"]`).click();
		const section = page.getByRole("button", { name: "Keyframes" });
		await expect(section).toBeVisible();
		const posted = recordEdits(page);

		// The first keyframe, at the playhead (frame 0), with the easing as picked.
		await page.getByLabel("Property", { exact: true }).selectOption("scale");
		await page.getByLabel("Value", { exact: true }).fill("1.5");
		await page
			.getByLabel("Easing", { exact: true })
			.selectOption("ease_in_out");
		await page.getByRole("button", { name: "Add at playhead" }).click();
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "keyframe.set",
				args: {
					item_id: itemId,
					property: "scale",
					at: "0f",
					value: 1.5,
					easing: "ease_in_out",
				},
			},
		]);
		await expect(page.locator("[data-keyframe-row]")).toHaveCount(1);
		await expect(page.locator("[data-keyframe-property=scale]")).toHaveCount(1);

		// Move the playhead ten frames on and add another one.
		const ruler = page.getByRole("slider", { name: "Playhead" });
		await ruler.focus();
		for (let i = 0; i < LATER; i++) await ruler.press("ArrowRight");
		await page.getByLabel("Value", { exact: true }).fill("2");
		await page.getByLabel("Easing", { exact: true }).selectOption("hold");
		await page.getByRole("button", { name: "Add at playhead" }).click();
		await expect.poll(() => posted.length).toBe(2);
		expect(posted[1]?.operations).toEqual([
			{
				op: "keyframe.set",
				args: {
					item_id: itemId,
					property: "scale",
					at: `${LATER}f`,
					value: 2,
					easing: "hold",
				},
			},
		]);
		await expect(page.locator("[data-keyframe-row]")).toHaveCount(2);
		await expect(page.locator("[data-keyframe-property=scale]")).toHaveCount(2);
		const keys = (await storedKeyframes(page, itemId)).scale ?? [];
		expect(keys.map((k) => k.easing)).toEqual(["ease_in_out", "hold"]);

		// A row's easing reaches the core as picked, in the registry spelling.
		await page
			.getByLabel(/^Easing of the Scale keyframe at/)
			.first()
			.selectOption("ease_out");
		await expect.poll(() => posted.length).toBe(3);
		expect(posted[2]?.operations[0]).toMatchObject({
			op: "keyframe.set",
			args: { property: "scale", at: "0f", value: 1.5, easing: "ease_out" },
		});

		// Delete on a selected diamond removes that keyframe only.
		await page.locator("[data-keyframe-property=scale]").last().click();
		await page.keyboard.press("Delete");
		await expect.poll(() => posted.length).toBe(4);
		expect(posted[3]?.operations).toEqual([
			{
				op: "keyframe.remove",
				args: { item_id: itemId, property: "scale", at: `${LATER}f` },
			},
		]);
		await expect(page.locator("[data-keyframe-row]")).toHaveCount(1);
		await expect(page.locator(`[data-item-id="${itemId}"]`)).toBeVisible();

		// Clear a property, then clear everything.
		await page
			.getByRole("button", { name: "Clear the keyframes of Scale" })
			.click();
		await expect.poll(() => posted.length).toBe(5);
		expect(posted[4]?.operations).toEqual([
			{
				op: "keyframe.clear",
				args: { item_id: itemId, property: "scale" },
			},
		]);
		await expect(page.locator("[data-keyframe-row]")).toHaveCount(0);

		await page.getByLabel("Property", { exact: true }).selectOption("opacity");
		await page.getByLabel("Value", { exact: true }).fill("0.5");
		await page.getByRole("button", { name: "Add at playhead" }).click();
		await expect.poll(() => posted.length).toBe(6);
		await page.getByRole("button", { name: "Clear all" }).click();
		await expect.poll(() => posted.length).toBe(7);
		expect(posted[6]?.operations).toEqual([
			{ op: "keyframe.clear", args: { item_id: itemId } },
		]);
		await expect(page.locator("[data-keyframe-row]")).toHaveCount(0);
		await expect(page.locator("[data-keyframe-property]")).toHaveCount(0);
	});

	test("with the playhead outside the item nothing can be added", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "1s",
		});
		const itemId = seeded.itemIds[0] as string;
		// The playhead opens far past the 1 s clip.
		await page.goto(`/editor/${seeded.id}?debug=1&t=300f`);
		await page.locator(`[data-item-id="${itemId}"]`).click();
		const add = page.getByRole("button", { name: "Add at playhead" });
		await expect(add).toBeDisabled();
		await expect(
			page.getByText("The playhead is outside this item."),
		).toBeVisible();
	});

	test("an animated Transform field shows a marker and the plan value, and sends keyframe.set", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${itemId}"]`).click();
		await page.getByLabel("Property", { exact: true }).selectOption("rotation");
		await page.getByLabel("Value", { exact: true }).fill("10");
		await page.getByRole("button", { name: "Add at playhead" }).click();
		await expect(page.locator("[data-keyframe-row]")).toHaveCount(1);
		const posted = recordEdits(page);

		const field = page.getByLabel("Rotation", { exact: true });
		await expect(field).toHaveValue("10");
		await field.fill("20");
		await field.press("Enter");

		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations[0]).toMatchObject({
			op: "keyframe.set",
			args: { item_id: itemId, property: "rotation", at: "0f", value: 20 },
		});
	});

	// VV-180: a refused keyframe shows the core's message and hint under the
	// button, and the field is marked invalid until the value changes.
	test("a keyframe the core refuses shows the error with its hint", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${itemId}"]`).click();
		await page.getByLabel("Property", { exact: true }).selectOption("hue");
		const value = page.getByLabel("Value", { exact: true });
		await value.fill("200");
		await page.getByRole("button", { name: "Add at playhead" }).click();
		const alert = page.getByRole("alert").filter({ hasText: "hue" });
		await expect(alert).toBeVisible();
		await expect(alert).toContainText("Use a value in range.");
		await expect(value).toHaveAttribute("aria-invalid", "true");
		await expect(page.locator("[data-keyframe-row]")).toHaveCount(0);

		await value.fill("20");
		await expect(alert).toHaveCount(0);
		await page.getByRole("button", { name: "Add at playhead" }).click();
		await expect(page.locator("[data-keyframe-row]")).toHaveCount(1);
	});

	// WCAG 2.5.8: the value input, the easing select and the buttons of a
	// keyframe row do not overlap and each has room to be pressed.
	test("a keyframe row passes the axe target-size rule", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${itemId}"]`).click();
		await page.getByLabel("Property", { exact: true }).selectOption("scale");
		await page.getByLabel("Value", { exact: true }).fill("1.5");
		await page.getByRole("button", { name: "Add at playhead" }).click();
		const row = page.locator("[data-keyframe-row]");
		await expect(row).toHaveCount(1);

		const { violations } = await new AxeBuilder({ page })
			.include("[data-keyframe-row]")
			.withRules(["target-size"])
			.analyze();
		expect(violations.map((v) => v.nodes.map((n) => n.failureSummary))).toEqual(
			[],
		);
	});
});
