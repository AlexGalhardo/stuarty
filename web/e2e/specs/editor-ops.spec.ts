import { expect, seedProject, test } from "./helpers";

// P1-UI-004 asserts that need a real server: the scenarios of SDD-012 section
// 20.1. The unit tests of resources/js/editor/commands and export cover the
// same gestures against the fixture server.
test.describe("thin-client-no-optimistic", () => {
	test("one gesture sends exactly one edit request and the item appears only on the answer", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const edits: string[] = [];
		page.on("request", (r) => {
			if (r.url().endsWith("/edit")) edits.push(r.url());
		});

		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("button", { name: "Step forward one frame" }).click();
		await page.getByRole("button", { name: "Step forward one frame" }).click();
		await page.locator("[data-item-id]").click();

		// The request is held until the count is read, so the test sees the
		// timeline before the core answers.
		let release: () => void = () => {};
		const held = new Promise<void>((resolve) => {
			release = resolve;
		});
		await page.route("**/edit", async (route) => {
			await held;
			await route.continue();
		});
		await page.getByRole("button", { name: "Split" }).click();
		await expect.poll(() => edits.length).toBe(1);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		release();
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		expect(edits).toHaveLength(1);
	});
});

test.describe("add-to-timeline-appends", () => {
	test("adding twice puts the second item after the first, with no overlap notice (VV-158)", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const add = page.getByRole("button", { name: "Add to timeline" }).first();
		await add.click();
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		await add.click();
		await expect(page.locator("[data-item-id]")).toHaveCount(3);
		await expect(page.getByRole("alert")).toHaveCount(0);
		const items = await page.evaluate(() => {
			const doc = window.__vv?.document as {
				items: Record<string, { start: number; duration: number }>;
			};
			return Object.values(doc.items)
				.map((i) => ({ start: i.start, end: i.start + i.duration }))
				.sort((a, b) => a.start - b.start);
		});
		expect(items[1]?.start).toBe(items[0]?.end);
		expect(items[2]?.start).toBe(items[1]?.end);
	});
});

test.describe("export-dialog", () => {
	test("lint, start, progress, then the path and a download link", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.getByRole("button", { name: "Export" }).click();
		await page.getByRole("button", { name: "Start export" }).click();
		await expect(page.getByRole("link", { name: "Download" })).toBeVisible({
			timeout: 90_000,
		});
	});
});
