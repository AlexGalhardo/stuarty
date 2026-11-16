import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

function recordEdits(page: Page): Posted[] {
	const edits: Posted[] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			edits.push(r.postDataJSON());
		}
	});
	return edits;
}

// TASK-0280 (SDD-012 sections 8.2 and 18.7, SDD-002 C.65 to C.67): link,
// unlink and extract audio from the item menu and the keyboard; a linked item
// shows a link mark.
test.describe("link-extract-audio", () => {
	test("the ~ key links the selected items, shows the mark and unlinks them", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(2);
		const edits = recordEdits(page);

		await items.nth(0).click();
		await items.nth(1).click({ modifiers: ["Shift"] });
		await page.keyboard.press("~");
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations).toHaveLength(1);
		expect(edits[0]?.operations[0]?.op).toBe("item.link");
		expect(
			(edits[0]?.operations[0]?.args.item_ids as string[]).slice().sort(),
		).toEqual([...seeded.itemIds].sort());
		await expect(page.locator("[data-badge=link]")).toHaveCount(2);
		const ids = await page
			.locator("[data-badge=link]")
			.evaluateAll((nodes) => nodes.map((n) => n.getAttribute("data-link-id")));
		expect(new Set(ids).size).toBe(1);

		await page.keyboard.press("~");
		await expect.poll(() => edits.length).toBe(2);
		expect(edits[1]?.operations[0]?.op).toBe("item.unlink");
		await expect(page.locator("[data-badge=link]")).toHaveCount(0);
	});

	test("the item menu links two items and unlinks them", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(2);
		const edits = recordEdits(page);

		await items.nth(0).click();
		await items.nth(1).click({ modifiers: ["Shift"] });
		await items.nth(1).click({ button: "right" });
		await expect(
			page.getByRole("menuitem", { name: "Unlink items", exact: true }),
		).toHaveCount(0);
		await page
			.getByRole("menuitem", { name: "Link items", exact: true })
			.click();
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations[0]?.op).toBe("item.link");
		await expect(page.locator("[data-badge=link]")).toHaveCount(2);

		await items.nth(1).click({ button: "right" });
		await expect(
			page.getByRole("menuitem", { name: "Link items", exact: true }),
		).toHaveCount(0);
		await page
			.getByRole("menuitem", { name: "Unlink items", exact: true })
			.click();
		await expect.poll(() => edits.length).toBe(2);
		expect(edits[1]?.operations[0]?.op).toBe("item.unlink");
		await expect(page.locator("[data-badge=link]")).toHaveCount(0);
	});

	test("Extract audio adds an audio track and a linked audio item and mutes the clip", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const clipId = seeded.itemIds[0] ?? "";
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(1);
		const edits = recordEdits(page);

		await items.first().click({ button: "right" });
		await page.getByRole("menuitem", { name: "Extract audio" }).click();
		await expect.poll(() => edits.length).toBe(1);
		const ops = edits[0]?.operations ?? [];
		expect(ops.map((o) => o.op)).toEqual(["track.add", "item.extract_audio"]);
		expect(ops[1]?.args.item_id).toBe(clipId);
		expect(ops[1]?.args.track_id).toBe(ops[0]?.args.id);

		await expect(items).toHaveCount(2);
		await expect(page.locator("[data-badge=link]")).toHaveCount(2);
		await expect
			.poll(async () =>
				page.evaluate(
					(id) =>
						(
							window.__vv?.document as {
								items: Record<string, { audio_muted?: boolean }>;
							}
						).items[id]?.audio_muted,
					clipId,
				),
			)
			.toBe(true);

		// Once muted the clip has nothing left to extract.
		await items.first().click({ button: "right" });
		await expect(
			page.getByRole("menuitem", { name: "Extract audio" }),
		).toHaveCount(0);
	});

	test("Mod+Shift+S sends the same extraction", async ({ page, request }) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(1);
		const edits = recordEdits(page);

		await items.first().click();
		await page.keyboard.press("Control+Shift+S");
		await expect.poll(() => edits.length).toBe(1);
		expect(edits[0]?.operations.map((o) => o.op)).toEqual([
			"track.add",
			"item.extract_audio",
		]);
		await expect(items).toHaveCount(2);
	});
});
