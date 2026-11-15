import type { Page } from "@playwright/test";
import { caller, expect, seedProject, test } from "./helpers";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

async function openSelected(
	page: Page,
	id: string,
	itemId: string,
): Promise<void> {
	await page.goto(`/editor/${id}?debug=1`);
	const item = page.locator("[data-item-id]");
	await expect(item).toHaveCount(1);
	const canvas = page.getByTestId("canvas-overlay");
	await expect(async () => {
		await canvas.click();
		await expect(item).toHaveAttribute("aria-selected", "true", {
			timeout: 500,
		});
	}).toPass();
	await expect(page.getByTestId("transform-handles")).toHaveAttribute(
		"data-item",
		itemId,
	);
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

async function centreOf(page: Page, selector: string) {
	const box = await page.locator(selector).first().boundingBox();
	if (box === null) throw new Error(`${selector} is not laid out`);
	return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

// A point of the last corner handle (bottom right) toward the inside of the
// box, so the pointer lands on the handle whatever lies around it.
async function cornerInside(page: Page) {
	const box = await page.locator("[data-handle=scale]").last().boundingBox();
	if (box === null) throw new Error("corner is not laid out");
	return { x: box.x + box.width / 4, y: box.y + box.height / 4 };
}

async function stored(
	page: Page,
	itemId: string,
): Promise<{ x?: number; y?: number; scale?: number; rotation?: number }> {
	return page.evaluate((id) => {
		const doc = window.__vv?.document as {
			items: Record<
				string,
				{ x?: number; y?: number; scale?: number; rotation?: number }
			>;
		};
		return doc.items[id] ?? {};
	}, itemId);
}

// P2-UI-016 (SDD-012 sections 8.2, 8.4 and 11.6): the handles move, scale and
// rotate the outline box under the pointer; one release sends one command; the
// picture changes only when the plan window of the new revision arrives.
test.describe("transform-handles", () => {
	test("a body drag sends one item.set_transform with x and y", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await openSelected(page, seeded.id, itemId);
		const posted = recordEdits(page);

		const from = await centreOf(page, "[data-handle=body]");
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + 30, from.y + 20, { steps: 6 });
		await page.mouse.up();

		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toHaveLength(1);
		expect(posted[0]?.operations[0]?.op).toBe("item.set_transform");
		expect(Object.keys(posted[0]?.operations[0]?.args ?? {}).sort()).toEqual([
			"item_id",
			"x",
			"y",
		]);
		await expect
			.poll(async () => (await stored(page, itemId)).x ?? 0)
			.toBeGreaterThan(0);
	});

	test("a corner drag sends one item.set_transform with scale", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await openSelected(page, seeded.id, itemId);
		const posted = recordEdits(page);

		const from = await cornerInside(page);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + 40, from.y + 40, { steps: 6 });
		await page.mouse.up();

		await expect.poll(() => posted.length).toBe(1);
		const op = posted[0]?.operations[0];
		expect(posted[0]?.operations).toHaveLength(1);
		expect(op?.op).toBe("item.set_transform");
		expect(Object.keys(op?.args ?? {}).sort()).toEqual(["item_id", "scale"]);
		await expect
			.poll(async () => (await stored(page, itemId)).scale ?? 1)
			.toBeGreaterThan(1);
	});

	test("the rotation handle sends one item.set_transform with rotation", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await openSelected(page, seeded.id, itemId);
		const posted = recordEdits(page);

		const handle = await centreOf(page, "[data-handle=rotate]");
		const body = await centreOf(page, "[data-handle=body]");
		await page.mouse.move(handle.x, handle.y);
		await page.mouse.down();
		await page.mouse.move(body.x + 150, body.y, { steps: 8 });
		await page.mouse.up();

		await expect.poll(() => posted.length).toBe(1);
		const op = posted[0]?.operations[0];
		expect(posted[0]?.operations).toHaveLength(1);
		expect(op?.op).toBe("item.set_transform");
		expect(Object.keys(op?.args ?? {}).sort()).toEqual(["item_id", "rotation"]);
		await expect
			.poll(async () => (await stored(page, itemId)).rotation ?? 0)
			.toBeGreaterThan(45);
	});

	test("the picture changes only after the new plan window arrives", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await openSelected(page, seeded.id, itemId);
		const posted = recordEdits(page);

		const shot = async (): Promise<string> => {
			// The overlay and the exact-frame image are not the picture.
			await page.evaluate(() => {
				for (const el of document.querySelectorAll("img, [data-testid]")) {
					(el as HTMLElement).style.visibility = "hidden";
				}
			});
			const buffer = await page
				.getByRole("main", { name: "Preview" })
				.locator("canvas")
				.screenshot();
			await page.evaluate(() => {
				for (const el of document.querySelectorAll("img, [data-testid]")) {
					(el as HTMLElement).style.visibility = "";
				}
			});
			return buffer.toString("base64");
		};

		const before = await shot();
		await page.route("**/plan?*", async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 1000));
			await route.continue();
		});

		const from = await cornerInside(page);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x - 60, from.y - 60, { steps: 6 });
		await page.mouse.up();
		await expect.poll(() => posted.length).toBe(1);

		// The plan window of the new revision is still on its way.
		expect(await shot()).toBe(before);
		await expect.poll(shot, { timeout: 10_000 }).not.toBe(before);
	});

	test("a property with keyframes is changed with keyframe.set at the playhead", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		const call = await caller(request);
		const head = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "keyframe.set",
					args: { item_id: itemId, property: "scale", at: "0.5s", value: 1.5 },
				},
			],
		});
		await openSelected(page, seeded.id, itemId);
		const posted = recordEdits(page);

		const from = await cornerInside(page);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + 40, from.y + 40, { steps: 6 });
		await page.mouse.up();

		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toHaveLength(1);
		expect(posted[0]?.operations[0]).toMatchObject({
			op: "keyframe.set",
			args: { item_id: itemId, property: "scale", at: "0f" },
		});
	});

	test("E_INVALID_STATE offers keyframes and the button resends the gesture as keyframe.set", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await openSelected(page, seeded.id, itemId);
		const posted = recordEdits(page);
		let refused = false;
		await page.route("**/edit", async (route) => {
			if (route.request().method() !== "POST" || refused) {
				await route.continue();
				return;
			}
			refused = true;
			await route.fulfill({
				status: 409,
				contentType: "application/json",
				json: {
					code: "E_INVALID_STATE",
					message: "scale has keyframes",
					hint: "Use keyframe.set or keyframe.clear.",
				},
			});
		});

		const from = await cornerInside(page);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + 40, from.y + 40, { steps: 6 });
		await page.mouse.up();

		await expect(page.getByRole("alert")).toContainText("scale has keyframes");
		await page.getByRole("button", { name: "Set as keyframe" }).click();
		await expect.poll(() => posted.length).toBe(2);
		expect(posted[1]?.operations).toHaveLength(1);
		expect(posted[1]?.operations[0]).toMatchObject({
			op: "keyframe.set",
			args: { item_id: itemId, property: "scale", at: "0f" },
		});
	});

	test("the inspector Transform section sends only the changed field", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await openSelected(page, seeded.id, itemId);
		const posted = recordEdits(page);

		const rotation = page.getByLabel("Rotation");
		await rotation.fill("15");
		await rotation.press("Enter");

		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{ op: "item.set_transform", args: { item_id: itemId, rotation: 15 } },
		]);
		await expect
			.poll(async () => (await stored(page, itemId)).rotation)
			.toBe(15);
	});

	test("a multi-selection shows mixed and sends one batch to every item", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		const [first, second] = seeded.itemIds as [string, string];
		const call = await caller(request);
		const head = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{ op: "item.set_transform", args: { item_id: first, rotation: 20 } },
			],
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(2);
		await items.first().click();
		await items.nth(1).click({ modifiers: ["Shift"] });
		const rotation = page.getByLabel("Rotation");
		await expect(rotation).toHaveAttribute("placeholder", "Mixed");
		const posted = recordEdits(page);

		await rotation.fill("45");
		await rotation.press("Enter");

		await expect.poll(() => posted.length).toBe(1);
		// The batch follows the timeline order of the selection, which the seeding
		// does not fix, so the order of the two operations is not asserted.
		const byItem = (a: { args: { item_id?: unknown } }, b: typeof a) =>
			String(a.args.item_id).localeCompare(String(b.args.item_id));
		expect([...(posted[0]?.operations ?? [])].sort(byItem)).toEqual(
			[
				{ op: "item.set_transform", args: { item_id: first, rotation: 45 } },
				{ op: "item.set_transform", args: { item_id: second, rotation: 45 } },
			].sort(byItem),
		);
	});
});
