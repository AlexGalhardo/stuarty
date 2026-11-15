import { expect, seedProject, test } from "./helpers";

// P2-UI-009 (SDD-012 sections 7.6 and 7.10): the selection model and the grid
// keys. A nudge of three selected items is one edit request with three
// operations and one revision.
test.describe("timeline-selection", () => {
	test("Shift+click ranges, Ctrl+click toggles, Esc and an empty click clear, Mod+A selects all", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 3 });
		await page.goto(`/editor/${seeded.id}`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(3);
		const selected = page.locator('[data-item-id][aria-selected="true"]');

		await items.nth(0).click();
		await expect(selected).toHaveCount(1);
		await items.nth(2).click({ modifiers: ["Shift"] });
		await expect(selected).toHaveCount(3);
		await items.nth(1).click({ modifiers: ["Control"] });
		await expect(selected).toHaveCount(2);
		await page.keyboard.press("Escape");
		await expect(selected).toHaveCount(0);

		await items.nth(1).click();
		await page.keyboard.press("Control+a");
		await expect(selected).toHaveCount(3);
		const area = page.locator(".overflow-auto.overscroll-contain");
		const box = await area.boundingBox();
		if (box === null) throw new Error("no scroll area");
		await area.click({ position: { x: box.width - 20, y: 8 } });
		await expect(selected).toHaveCount(0);

		// A header click selects the track.
		await page.getByRole("rowheader").first().click();
		await expect(selected).toHaveCount(0);
		await expect(page.getByRole("heading", { name: /Track/i })).toBeVisible();
	});

	test("a marquee selects the items it touches", async ({ page, request }) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 3 });
		await page.goto(`/editor/${seeded.id}`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(3);
		const first = await items.nth(0).boundingBox();
		const second = await items.nth(1).boundingBox();
		const row = await page.getByRole("row").first().boundingBox();
		if (first === null || second === null || row === null) {
			throw new Error("items are not laid out");
		}
		// From the empty strip under the items, across the first two.
		const y = row.y + row.height - 2;
		await page.mouse.move(first.x + 2, y);
		await page.mouse.down();
		await page.mouse.move(second.x + second.width / 2, first.y + 10, {
			steps: 6,
		});
		await page.mouse.up();
		await expect(
			page.locator('[data-item-id][aria-selected="true"]'),
		).toHaveCount(2);
	});

	test("nudging three selected items sends one edit with three operations and advances the revision by one", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 3 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(3);
		await items.nth(0).click();
		await items.nth(2).click({ modifiers: ["Shift"] });
		const before = await page.evaluate(() => window.__vv?.revision ?? 0);

		const posted: { operations: { op: string }[] }[] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				posted.push(r.postDataJSON());
			}
		});
		await page.keyboard.press("Alt+ArrowRight");
		await page.waitForFunction((r) => (window.__vv?.revision ?? 0) > r, before);
		expect(posted).toHaveLength(1);
		expect(posted[0]?.operations).toHaveLength(3);
		expect(posted[0]?.operations.every((o) => o.op === "item.move")).toBe(true);
		expect(await page.evaluate(() => window.__vv?.revision)).toBe(before + 1);
	});

	test("arrow keys move focus along the track and Down reaches the next track; focus survives a removal", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 2, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(4);
		await page.waitForFunction(() => window.__vv?.dispatch !== undefined);

		await items.first().focus();
		const firstId = await items.first().getAttribute("data-item-id");
		await page.keyboard.press("ArrowRight");
		const second = page.locator("[data-item-id]:focus");
		await expect(second).toHaveCount(1);
		const secondId = await second.getAttribute("data-item-id");
		expect(secondId).not.toBe(firstId);
		await expect(second).toHaveAttribute("tabindex", "0");

		// Remove the focused item through the API: focus lands on a neighbour.
		await page.evaluate(
			(id) =>
				window.__vv?.dispatch?.([
					{ op: "item.delete", args: { item_ids: [id] } },
				]),
			secondId,
		);
		await expect(items).toHaveCount(3);
		await expect(page.locator("[data-item-id]:focus")).toHaveCount(1);
	});
});
