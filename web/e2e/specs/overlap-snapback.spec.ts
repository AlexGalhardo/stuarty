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

// P2-UI-010 (SDD-012 section 8.4, Move): a drag that overlaps is refused by the
// core, the ghost goes, the item stays where the core has it, the item it hit
// is outlined and the notice offers the core's own next free place.
test.describe("overlap-snapback", () => {
	test("a drag onto a neighbour snaps back, outlines the conflict and offers the next free place", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(2);
		const before = await starts(page);
		const secondId = await items.nth(1).getAttribute("data-item-id");
		const firstId = await items.nth(0).getAttribute("data-item-id");
		if (secondId === null || firstId === null) throw new Error("no ids");

		const edits: {
			operations: { op: string; args: Record<string, unknown> }[];
		}[] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				edits.push(r.postDataJSON());
			}
		});
		const refused = page.waitForResponse(
			(r) => r.url().endsWith("/edit") && r.status() >= 400,
		);

		const from = await center(page, 1);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x - 14, from.y, { steps: 8 });
		await expect(page.getByTestId("ghost-layer")).toBeVisible();
		await page.mouse.up();
		const body = (await (await refused).json()) as {
			error?: {
				code: string;
				details?: { next_free?: number; conflicting_item?: string };
			};
			code?: string;
			details?: { next_free?: number; conflicting_item?: string };
		};
		const details = body.error?.details ?? body.details;

		await expect(page.getByRole("alert")).toContainText("Overlaps");
		await expect(page.getByTestId("ghost-layer")).toHaveCount(0);
		expect(await starts(page)).toEqual(before);
		expect(details?.conflicting_item).toBe(firstId);
		await expect(page.locator(`[data-item-id="${firstId}"]`)).toHaveAttribute(
			"data-conflicting",
			"",
		);

		const action = page.getByRole("button", {
			name: "Move to next free position",
		});
		expect(details?.next_free).toBeDefined();
		await action.click();
		await expect.poll(() => edits.length).toBe(2);
		expect(edits[1]?.operations[0]?.args.start).toBe(`${details?.next_free}f`);
	});
});
