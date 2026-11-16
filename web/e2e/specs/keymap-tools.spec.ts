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

async function open(page: Page, id: string, count: number): Promise<void> {
	await page.goto(`/editor/${id}?debug=1`);
	await expect(page.locator("[data-item-id]")).toHaveCount(count);
}

// TASK-0236 (SDD-012 section 18.7), the second part: the tools, the range keys,
// the keyframe panel key and the wheel. One test per key.
test.describe("compatible keymap, tools and ranges", () => {
	test("B arms the blade, a click cuts the item there, A and Esc go back to select", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		await open(page, seeded.id, 1);
		const edits = recordEdits(page);
		const blade = page.getByRole("button", { name: "Blade" });
		await expect(blade).toHaveAttribute("aria-pressed", "false");

		await page.locator("[data-item-id]").first().click();
		await page.keyboard.press("b");
		await expect(blade).toHaveAttribute("aria-pressed", "true");
		const cell = page.locator("[data-item-id]").first();
		const box = await cell.boundingBox();
		if (box === null) throw new Error("no item box");
		await page.mouse.click(box.x + box.width / 2, box.y + box.height / 2);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		expect(edits).toHaveLength(1);
		expect(edits[0]?.operations.map((o) => o.op)).toEqual(["item.split"]);
		const at = edits[0]?.operations[0]?.args.at as string | number;
		expect(at).toBeDefined();

		await page.keyboard.press("a");
		await expect(blade).toHaveAttribute("aria-pressed", "false");
		await page.keyboard.press("b");
		await expect(blade).toHaveAttribute("aria-pressed", "true");
		await page.keyboard.press("Escape");
		await expect(blade).toHaveAttribute("aria-pressed", "false");
	});

	test("Shift+X sets the in and out points to the selected item and Alt+X clears them", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 2,
			clipDuration: "1s",
		});
		await open(page, seeded.id, 2);
		const edits = recordEdits(page);
		const range = page.locator('[data-testid="ruler-range"]');
		await expect(range).toHaveCount(0);
		await page.locator("[data-item-id]").nth(1).click();
		await page.keyboard.press("Shift+X");
		await expect(range).toBeVisible();
		const [rangeBox, itemBox] = await Promise.all([
			range.boundingBox(),
			page.locator("[data-item-id]").nth(1).boundingBox(),
		]);
		expect(Math.abs((rangeBox?.x ?? 0) - (itemBox?.x ?? 99999))).toBeLessThan(
			3,
		);
		expect(
			Math.abs((rangeBox?.width ?? 0) - (itemBox?.width ?? 99999)),
		).toBeLessThan(3);
		await page.keyboard.press("Alt+x");
		await expect(range).toHaveCount(0);
		expect(edits).toHaveLength(0);
	});

	test("Alt+K shows the keyframes of the selected item and Alt+K again hides them", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await open(page, seeded.id, 1);
		await page.locator("[data-item-id]").first().click();
		const header = page.getByRole("button", { name: "Keyframes" });
		await header.click();
		await expect(header).toHaveAttribute("aria-expanded", "false");
		await page.locator("[data-item-id]").first().click();
		await page.keyboard.press("Alt+k");
		await expect(header).toHaveAttribute("aria-expanded", "true");
		await expect(header).toBeFocused();
		await page.keyboard.press("Alt+k");
		await expect(header).toHaveAttribute("aria-expanded", "false");
	});

	test("Ctrl+wheel zooms the timeline", async ({ page, request }) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await open(page, seeded.id, 1);
		const item = page.locator("[data-item-id]").first();
		const before = (await item.boundingBox())?.width ?? 0;
		const box = await page.locator("#timeline").boundingBox();
		if (box === null) throw new Error("no timeline");
		await page.mouse.move(box.x + 200, box.y + 60);
		await page.keyboard.down("Control");
		await page.mouse.wheel(0, -200);
		await page.keyboard.up("Control");
		await expect
			.poll(async () => (await item.boundingBox())?.width ?? 0)
			.toBeGreaterThan(before);
	});

	test("keys typed in a field do nothing", async ({ page, request }) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await open(page, seeded.id, 1);
		await page.locator("[data-item-id]").first().click();
		const blade = page.getByRole("button", { name: "Blade" });
		await page.getByRole("textbox").first().focus();
		await page.keyboard.press("b");
		await expect(blade).toHaveAttribute("aria-pressed", "false");
	});
});
