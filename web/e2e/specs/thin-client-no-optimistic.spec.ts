import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

async function center(
	page: Page,
	nth: number,
): Promise<{ x: number; y: number }> {
	const box = await page.locator("[data-item-id]").nth(nth).boundingBox();
	if (box === null) throw new Error("item is not laid out");
	return { x: box.x + box.width / 2, y: box.y + box.height / 2 };
}

// P2-UI-010 (SDD-012 section 1.2, T2 to T4): while the answer of the core is
// held, a released drag leaves the store alone and the ghost shows the request.
test.describe("thin-client-no-optimistic", () => {
	test("a drag with the answer held 1 s leaves the revision and the stored start unchanged until it arrives", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const read = () =>
			page.evaluate(() => {
				const doc = window.__vv?.document as {
					items: Record<string, { start: number }>;
				};
				return {
					revision: window.__vv?.revision,
					start: Object.values(doc.items)[0]?.start,
				};
			});
		const before = await read();

		await page.route("**/edit", async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 1000));
			await route.continue();
		});
		const from = await center(page, 0);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + 90, from.y, { steps: 8 });
		await page.mouse.up();

		await expect(
			page.locator('[data-ghost-id][data-pending="true"]'),
		).toBeVisible();
		expect(await read()).toEqual(before);

		await expect(page.getByTestId("ghost-layer")).toHaveCount(0);
		const after = await read();
		expect(after.revision).toBe((before.revision ?? 0) + 1);
		expect(after.start).toBeGreaterThan(before.start ?? 0);
	});

	test("dragging a selection of five items sends one edit with five operations", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 5 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(5);
		const edits: { operations: { op: string }[] }[] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				edits.push(r.postDataJSON());
			}
		});
		await page.locator("[data-item-id]").first().click();
		await page.keyboard.press("Control+a");
		await expect(
			page.locator('[data-item-id][aria-selected="true"]'),
		).toHaveCount(5);

		const from = await center(page, 2);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + 60, from.y, { steps: 8 });
		await page.mouse.up();
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toHaveLength(5);
		expect(edits[0]?.operations.every((o) => o.op === "item.move")).toBe(true);
	});

	test("Esc during a drag removes the ghost and sends nothing", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const edits: string[] = [];
		page.on("request", (r) => {
			if (r.url().endsWith("/edit")) edits.push(r.url());
		});
		const from = await center(page, 0);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + 80, from.y, { steps: 6 });
		await expect(page.getByTestId("ghost-layer")).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(page.getByTestId("ghost-layer")).toHaveCount(0);
		await page.mouse.up();
		expect(edits).toHaveLength(0);
	});

	test("Delete with the answer held 1 s leaves the item in the read model until it arrives", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(2);
		const count = () =>
			page.evaluate(
				() =>
					Object.keys(
						(
							(window.__vv?.document ?? { items: {} }) as {
								items: Record<string, unknown>;
							}
						).items,
					).length,
			);
		await page.route("**/edit", async (route) => {
			await new Promise((resolve) => setTimeout(resolve, 1000));
			await route.continue();
		});
		await items.first().click();
		await page.keyboard.press("Delete");
		await page.waitForTimeout(300);
		expect(await count()).toBe(2);
		await expect(items).toHaveCount(1);
		expect(await count()).toBe(1);
	});
});
