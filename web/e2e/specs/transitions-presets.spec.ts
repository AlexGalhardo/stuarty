import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";
import { openInspectorTab } from "./sections";

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

interface Stored {
	transition_in?: { kind: string; duration: number };
	transition_out?: { kind: string; duration: number };
}

async function stored(page: Page, itemId: string): Promise<Stored> {
	return page.evaluate((id) => {
		const doc = window.__vv?.document as { items: Record<string, Stored> };
		return doc.items[id] ?? {};
	}, itemId);
}

// P2-UI-021 (SDD-012 sections 8.2 and 12.1): transitions, the crossfade menu
// entry and the animation presets. Each control sends one command; the picture
// of a fade is the plan window's (the golden case is P2-QA-003).
test.describe("transitions-presets", () => {
	test("each transition control sends one item.set_transition", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${itemId}"]`).click();
		const posted = recordEdits(page);

		await page
			.getByLabel("In transition", { exact: true })
			.selectOption("fade");
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "item.set_transition",
				args: { item_id: itemId, edge: "in", kind: "fade" },
			},
		]);
		await expect(page.locator("[data-transition-mark=in]")).toHaveCount(1);
		await expect
			.poll(async () => (await stored(page, itemId)).transition_in?.kind)
			.toBe("fade");

		const length = page.getByLabel("In duration", { exact: true });
		await length.fill("0.3s");
		await length.press("Enter");
		await expect.poll(() => posted.length).toBe(2);
		expect(posted[1]?.operations).toEqual([
			{
				op: "item.set_transition",
				args: { item_id: itemId, edge: "in", kind: "fade", duration: "0.3s" },
			},
		]);

		// A new kind keeps the length the read model holds.
		await page
			.getByLabel("In transition", { exact: true })
			.selectOption("slide_up");
		await expect.poll(() => posted.length).toBe(3);
		expect(posted[2]?.operations[0]).toMatchObject({
			op: "item.set_transition",
			args: { item_id: itemId, edge: "in", kind: "slide_up" },
		});
		expect(typeof posted[2]?.operations[0]?.args.duration).toBe("string");

		await page
			.getByLabel("Out transition", { exact: true })
			.selectOption("slide_left");
		await expect.poll(() => posted.length).toBe(4);
		expect(posted[3]?.operations).toEqual([
			{
				op: "item.set_transition",
				args: { item_id: itemId, edge: "out", kind: "slide_left" },
			},
		]);
		await expect(page.locator("[data-transition-mark=out]")).toHaveCount(1);

		// None removes it with a null kind.
		await page.getByLabel("In transition", { exact: true }).selectOption("");
		await expect.poll(() => posted.length).toBe(5);
		expect(posted[4]?.operations).toEqual([
			{
				op: "item.set_transition",
				args: { item_id: itemId, edge: "in", kind: null },
			},
		]);
		await expect(page.locator("[data-transition-mark=in]")).toHaveCount(0);
	});

	test("the animation section sends one text.apply_preset", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${itemId}"]`).click();
		const posted = recordEdits(page);

		await openInspectorTab(page, "Animation");
		await page.getByLabel("Preset", { exact: true }).selectOption("pop");
		await page.getByLabel("Edge", { exact: true }).selectOption("out");
		await page.getByRole("button", { name: "Apply" }).click();
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "text.apply_preset",
				args: { item_id: itemId, edge: "out", preset: "pop" },
			},
		]);

		await page.getByLabel("Duration", { exact: true }).fill("0.5s");
		await page.getByRole("button", { name: "Apply" }).click();
		await expect.poll(() => posted.length).toBe(2);
		expect(posted[1]?.operations).toEqual([
			{
				op: "text.apply_preset",
				args: { item_id: itemId, edge: "out", preset: "pop", duration: "0.5s" },
			},
		]);
	});

	test("Crossfade with two items selected sends one item.crossfade from the earlier item", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		// `created` is not in timeline order: read which item starts first.
		const starts = await page.evaluate(() => {
			const doc = window.__vv?.document as {
				items: Record<string, { start: number }>;
			};
			return Object.entries(doc.items)
				.map(([id, item]) => ({ id, start: item.start }))
				.sort((x, y) => x.start - y.start);
		});
		const [first, second] = starts.map((entry) => entry.id) as [string, string];
		await page.locator(`[data-item-id="${second}"]`).click();
		await page
			.locator(`[data-item-id="${first}"]`)
			.click({ modifiers: ["Control"] });
		const posted = recordEdits(page);

		await page.locator(`[data-item-id="${first}"]`).click({ button: "right" });
		await page.getByRole("menuitem", { name: "Crossfade" }).click();

		await expect.poll(() => posted.length).toBe(1);
		// Both items touch on one track, so the same batch adds the track the
		// arriving item moves to (VV-042).
		const operations = posted[0]?.operations as Array<{
			op: string;
			args: { id?: string };
		}>;
		const added = operations[0]?.args.id as string;
		expect(added).toMatch(/^trk_/);
		expect(operations).toEqual([
			{ op: "track.add", args: { kind: "visual", id: added } },
			{
				op: "item.crossfade",
				args: {
					from_item_id: first,
					to_item_id: second,
					duration: "0.5s",
					track_id: added,
				},
			},
		]);
	});

	test("E_INVALID_STATE on a transition shows the hint and changes nothing", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${itemId}"]`).click();
		await page.route("**/edit", async (route) => {
			if (route.request().method() !== "POST") {
				await route.continue();
				return;
			}
			await route.fulfill({
				status: 409,
				contentType: "application/json",
				json: {
					code: "E_INVALID_STATE",
					message: "The transition no longer fits the item.",
					hint: "Use a shorter transition.",
				},
			});
		});

		await page
			.getByLabel("In transition", { exact: true })
			.selectOption("fade");

		await expect(page.getByRole("alert")).toContainText(
			"Use a shorter transition.",
		);
		expect(await stored(page, itemId)).toEqual(
			expect.not.objectContaining({ transition_in: expect.anything() }),
		);
		await expect(page.locator("[data-transition-mark]")).toHaveCount(0);
	});
});
