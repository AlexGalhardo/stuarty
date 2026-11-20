import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

interface StoredKey {
	t: number;
	value: number;
	easing?: string;
	bezier?: number[];
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

async function storedKeys(
	page: Page,
	itemId: string,
	property: string,
): Promise<StoredKey[]> {
	return page.evaluate(
		([id, prop]) => {
			const doc = window.__vv?.document as {
				items: Record<string, { keyframes?: Record<string, StoredKey[]> }>;
			};
			return doc.items[id as string]?.keyframes?.[prop as string] ?? [];
		},
		[itemId, property],
	);
}

const LATER = 10;

// Epic 31 (RF-080, RF-081, RF-082; SDD-012 section 12.3): the curve editor of a
// keyframe segment, the crop edges keyframed from the inspector and the motion
// presets. Every action is one command; the browser works out no value.
test.describe("motion and easing", () => {
	test("edit a curve: a custom curve, handles, a named curve and the four numbers", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${itemId}"]`).click();

		await page.getByLabel("Property", { exact: true }).selectOption("scale");
		await page.getByLabel("Value", { exact: true }).fill("1");
		await page.getByRole("button", { name: "Add at playhead" }).click();
		const ruler = page.getByRole("slider", { name: "Playhead" });
		await ruler.focus();
		for (let i = 0; i < LATER; i++) await ruler.press("ArrowRight");
		await page.getByLabel("Value", { exact: true }).fill("2");
		await page.getByRole("button", { name: "Add at playhead" }).click();
		await expect(page.locator("[data-keyframe-row]")).toHaveCount(2);
		const posted = recordEdits(page);

		// "Custom curve" starts from the named curve in_out and opens the editor.
		await page
			.getByLabel(/^Easing of the Scale keyframe at/)
			.first()
			.selectOption("bezier");
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations[0]).toMatchObject({
			op: "keyframe.set_easing",
			args: { property: "scale", at: "0f", preset: "in_out" },
		});
		await expect(page.locator("[data-bezier-editor=scale]")).toBeVisible();
		await expect
			.poll(async () => (await storedKeys(page, itemId, "scale"))[0]?.bezier)
			.toEqual([0.42, 0, 0.58, 1]);

		// Type one number: the whole curve goes, with that number changed.
		const x1 = page.getByLabel(/^x1 of the Scale/);
		await x1.fill("0.2");
		await x1.press("Enter");
		await expect.poll(() => posted.length).toBe(2);
		expect(posted[1]?.operations[0]).toMatchObject({
			op: "keyframe.set_easing",
			args: { property: "scale", at: "0f", bezier: [0.2, 0, 0.58, 1] },
		});

		await expect
			.poll(async () => (await storedKeys(page, itemId, "scale"))[0]?.bezier)
			.toEqual([0.2, 0, 0.58, 1]);

		// A handle moves with the arrow keys; the gesture ends on key up.
		const handle = page.getByRole("slider", {
			name: "Control point 2 of the curve",
		});
		await handle.focus();
		await handle.press("ArrowUp");
		await expect.poll(() => posted.length).toBe(3);
		expect(posted[2]?.operations[0]).toMatchObject({
			op: "keyframe.set_easing",
			args: { property: "scale", at: "0f", bezier: [0.2, 0, 0.58, 1.01] },
		});

		await expect
			.poll(async () => (await storedKeys(page, itemId, "scale"))[0]?.bezier)
			.toEqual([0.2, 0, 0.58, 1.01]);

		// A handle moves with the pointer.
		const box = await handle.boundingBox();
		if (box === null) throw new Error("the handle has no box");
		const from = { x: box.x + box.width / 2, y: box.y + box.height / 2 };
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x - 20, from.y - 20, { steps: 4 });
		await page.mouse.up();
		await expect.poll(() => posted.length).toBe(4);
		const dragged = posted[3]?.operations[0]?.args.bezier as number[];
		expect(dragged).toHaveLength(4);
		expect(dragged[2]).toBeLessThan(0.58);
		expect(dragged[3]).toBeGreaterThan(1.01);

		// A named curve is stored as its four numbers.
		await page
			.getByLabel(/^Named curve for the Scale .* keyframe/)
			.selectOption("overshoot");
		await expect.poll(() => posted.length).toBe(5);
		expect(posted[4]?.operations[0]).toMatchObject({
			op: "keyframe.set_easing",
			args: { property: "scale", at: "0f", preset: "overshoot" },
		});
		await expect
			.poll(async () => (await storedKeys(page, itemId, "scale"))[0]?.bezier)
			.toEqual([0.34, 1.56, 0.64, 1]);

		// A new value keeps the curve of the keyframe.
		await page
			.getByLabel(/^Value of the Scale keyframe at/)
			.first()
			.fill("1.5");
		await page
			.getByLabel(/^Value of the Scale keyframe at/)
			.first()
			.press("Enter");
		await expect.poll(() => posted.length).toBe(6);
		expect(posted[5]?.operations[0]).toMatchObject({
			op: "keyframe.set",
			args: {
				property: "scale",
				value: 1.5,
				easing: "bezier",
				bezier: [0.34, 1.56, 0.64, 1],
			},
		});

		// The editor has no accessibility violation.
		const results = await new AxeBuilder({ page })
			.include("[data-bezier-editor=scale]")
			.analyze();
		expect(results.violations).toEqual([]);

		// A named easing clears the curve.
		await page
			.getByLabel(/^Easing of the Scale keyframe at/)
			.first()
			.selectOption("hold");
		await expect.poll(() => posted.length).toBe(7);
		await expect
			.poll(async () => (await storedKeys(page, itemId, "scale"))[0]?.easing)
			.toBe("hold");
		expect(
			(await storedKeys(page, itemId, "scale"))[0]?.bezier,
		).toBeUndefined();
	});

	test("a crop edge is keyframed from the inspector", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const itemId = seeded.itemIds[0] as string;
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${itemId}"]`).click();
		const add = page.getByRole("button", {
			name: "Add a keyframe for Crop left at the playhead",
		});
		await expect(add).toBeEnabled();
		const posted = recordEdits(page);
		await add.click();
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "keyframe.set",
				args: { item_id: itemId, property: "crop_left", at: "0f", value: 0 },
			},
		]);
		await expect(page.locator("[data-keyframe-row]")).toHaveCount(1);
		await expect(
			page.getByRole("button", {
				name: "Remove the keyframe of Crop left at the playhead",
			}),
		).toBeVisible();
	});

	test("a motion preset sends one command and its keyframes are ordinary ones", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const itemId = seeded.itemIds[0] as string;
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${itemId}"]`).click();
		const posted = recordEdits(page);

		const tabs = page.getByRole("tablist", { name: "Properties" });
		await tabs.getByRole("tab", { name: "Animation" }).click();
		// The Animation section shares the tab: every control has a name of its own.
		await page
			.getByLabel("Motion preset", { exact: true })
			.selectOption("slide");
		await page.getByLabel("Direction", { exact: true }).selectOption("up");
		await page.getByRole("button", { name: "Add motion" }).click();
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "animation.apply_preset",
				args: {
					item_id: itemId,
					preset: "slide",
					edge: "in",
					direction: "up",
				},
			},
		]);
		await tabs.getByRole("tab", { name: "Video" }).click();
		await expect(page.locator("[data-keyframe-row]")).toHaveCount(2);
		await expect(page.locator("[data-keyframe-row^='y:']")).toHaveCount(2);
		const keys = await storedKeys(page, itemId, "y");
		expect(keys.map((k) => k.easing)).toEqual(["bezier", "linear"]);

		// A preset longer than the item is refused by the core and changes nothing.
		await tabs.getByRole("tab", { name: "Animation" }).click();
		await page
			.getByLabel("Duration of the motion preset", { exact: true })
			.fill("60s");
		await page.getByRole("button", { name: "Add motion" }).click();
		await expect.poll(() => posted.length).toBe(2);
		expect(await storedKeys(page, itemId, "y")).toHaveLength(2);
		// The refusal shows under the section as the core sent it.
		await expect(
			page.getByRole("alert").filter({ hasText: /lasts \d+/ }),
		).toBeVisible();
	});
});
