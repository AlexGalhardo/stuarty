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

async function starts(page: Page): Promise<Record<string, number>> {
	return page.evaluate(() => {
		const doc = window.__vv?.document as {
			items: Record<string, { start: number }>;
		};
		return Object.fromEntries(
			Object.entries(doc.items).map(([id, item]) => [id, item.start]),
		);
	});
}

interface Diff {
	diff: (
		| { kind: "item.removed"; id: string; object: { duration: number } }
		| {
				kind: "item.updated";
				id: string;
				changes: { start?: [number, number] };
		  }
	)[];
}

// P2-UI-013 (SDD-012 sections 8.2 and 7.10): delete, ripple delete, duplicate,
// close gap and rename each send one batch, and the read model changes only
// when the core has answered.
test.describe("delete-ripple-duplicate", () => {
	test("ripple delete from the menu moves the later items by the freed length the core reports", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 3 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(3);
		const before = await starts(page);
		const edits = recordEdits(page);
		const answered = page.waitForResponse(
			(r) => r.url().endsWith("/edit") && r.status() === 200,
		);

		await items.nth(0).click({ button: "right" });
		await page.getByRole("menuitem", { name: "Ripple delete" }).first().click();
		const body = (await (await answered).json()) as Diff;

		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations).toHaveLength(1);
		expect(edits[0]?.operations[0]?.op).toBe("item.delete");
		expect(edits[0]?.operations[0]?.args.ripple).toBe("track");
		const removed = body.diff.find((c) => c.kind === "item.removed");
		if (removed?.kind !== "item.removed") throw new Error("no removal in diff");
		await expect(items).toHaveCount(2);
		const after = await starts(page);
		const moved = body.diff.flatMap((c) =>
			c.kind === "item.updated" && c.changes.start !== undefined
				? [{ id: c.id, from: c.changes.start[0], to: c.changes.start[1] }]
				: [],
		);
		expect(moved).toHaveLength(2);
		for (const m of moved) {
			expect(m.from).toBe(before[m.id]);
			expect(m.from - m.to).toBe(removed.object.duration);
			expect(after[m.id]).toBe(m.to);
		}
	});

	test("Delete removes the selection in one batch and focus moves to the next item", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 3 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(3);
		const second = await items.nth(1).getAttribute("data-item-id");
		const third = await items.nth(2).getAttribute("data-item-id");
		const edits = recordEdits(page);

		await items.nth(1).focus();
		await items.nth(1).click();
		await page.keyboard.press("Delete");

		await expect(items).toHaveCount(2);
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations).toEqual([
			{ op: "item.delete", args: { item_ids: [second] } },
		]);
		await expect(page.locator(`[data-item-id="${third}"]`)).toBeFocused();
	});

	test("Ctrl+D sends one item.duplicate and the core places it right after the item end", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(1);
		const edits = recordEdits(page);

		await items.first().click();
		await page.keyboard.press("Control+d");

		await expect(items).toHaveCount(2);
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations).toHaveLength(1);
		expect(edits[0]?.operations[0]?.op).toBe("item.duplicate");
		// No start in the request: the core picks the first free position.
		expect(edits[0]?.operations[0]?.args.start).toBeUndefined();
		const doc = await page.evaluate(
			() =>
				(
					(window.__vv?.document ?? { items: {} }) as {
						items: Record<string, { start: number; duration: number }>;
					}
				).items,
		);
		const [source, copy] = Object.values(doc).sort((a, b) => a.start - b.start);
		expect(copy?.start).toBe((source?.start ?? 0) + (source?.duration ?? 0));
	});

	test("the gap menu sends one timeline.close_gap and the later item moves earlier", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 3 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(3);
		const last = await items.nth(2).getAttribute("data-item-id");
		if (last === null) throw new Error("no id");
		await items.nth(1).click();
		await page.keyboard.press("Delete");
		await expect(items).toHaveCount(2);
		const before = (await starts(page))[last] ?? 0;
		const edits = recordEdits(page);

		const left = await items.nth(0).boundingBox();
		const right = await items.nth(1).boundingBox();
		if (left === null || right === null) throw new Error("not laid out");
		const x = (left.x + left.width + right.x) / 2;
		await page.mouse.click(x, left.y + left.height / 2, { button: "right" });
		await page.getByRole("menuitem", { name: "Close gap" }).click();

		await expect
			.poll(async () => (await starts(page))[last])
			.toBeLessThan(before);
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations).toHaveLength(1);
		expect(edits[0]?.operations[0]?.op).toBe("timeline.close_gap");
		expect(String(edits[0]?.operations[0]?.args.at)).toMatch(/^\d+f$/);
	});

	test("F2 renames inline with one item.rename; Esc sends nothing", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(2);
		const edits = recordEdits(page);

		await items.nth(0).click();
		await page.keyboard.press("F2");
		const field = page.getByRole("textbox", { name: "Name of the item" });
		await field.fill("ignored");
		await page.keyboard.press("Escape");
		await expect(field).toHaveCount(0);
		expect(edits).toHaveLength(0);

		await page.keyboard.press("F2");
		await page.getByRole("textbox", { name: "Name of the item" }).fill("Intro");
		await page.keyboard.press("Enter");
		await expect(items.nth(0)).toHaveAttribute("aria-label", /Intro/);
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations).toHaveLength(1);
		expect(edits[0]?.operations[0]?.op).toBe("item.rename");
		expect(edits[0]?.operations[0]?.args.name).toBe("Intro");
	});
});
