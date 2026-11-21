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

// P2-UI-026 (SDD-012 sections 8.2 and 12.1): shapes are added from the toolbar
// menu and edited in the inspector, one command each, and the preview draws
// them through a bitmap layer of the plan.
test.describe("shapes", () => {
	test("add a shape and edit its fields, one command each; the preview loads a plan bitmap", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		const bitmaps: string[] = [];
		page.on("request", (r) => {
			if (r.url().includes("/bitmaps/")) bitmaps.push(r.url());
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.getByRole("main", { name: "Preview" })).toBeVisible();
		const posted = recordEdits(page);

		await page.getByRole("tab", { name: "Text" }).click();
		await page.getByRole("button", { name: "Rounded rectangle" }).click();
		await expect.poll(() => posted.length).toBe(1);
		expect(posted[0]?.operations).toEqual([
			{
				op: "shape.add",
				// No track_id: the core picks a free visual track (SDD-002 C.6).
				args: {
					start: "0f",
					duration: "3s",
					shape: "rounded_rect",
					width: 400,
					height: 240,
				},
			},
		]);
		const item = page.locator("[data-item-id]");
		await expect(item).toHaveCount(1);
		await expect.poll(() => bitmaps.length).toBeGreaterThan(0);

		await item.click();
		await expect(
			page.getByRole("heading", { name: "Shape", exact: true }),
		).toBeVisible();
		const itemId = (await item.getAttribute("data-item-id")) ?? "";

		await openInspectorTab(page, "Text");
		const width = page.getByLabel("Width", { exact: true });
		await width.fill("600");
		await width.press("Enter");
		await expect.poll(() => posted.length).toBe(2);
		const radius = page.getByLabel("Corner radius", { exact: true });
		await radius.fill("24");
		await radius.press("Enter");
		await expect.poll(() => posted.length).toBe(3);
		const fill = page.getByLabel("Fill", { exact: true });
		await fill.fill("#FF0000");
		await fill.press("Enter");
		await expect.poll(() => posted.length).toBe(4);
		const strokeWidth = page.getByLabel("Stroke width", { exact: true });
		await strokeWidth.fill("4");
		await strokeWidth.press("Enter");
		await expect.poll(() => posted.length).toBe(5);
		await page.getByLabel("Shape", { exact: true }).selectOption("ellipse");
		await expect.poll(() => posted.length).toBe(6);

		expect(posted.slice(1).map((p) => p.operations)).toEqual([
			[{ op: "shape.set", args: { item_id: itemId, width: 600 } }],
			[{ op: "shape.set", args: { item_id: itemId, radius: 24 } }],
			[{ op: "shape.set", args: { item_id: itemId, fill: "#FF0000" } }],
			[{ op: "shape.set", args: { item_id: itemId, stroke_width: 4 } }],
			[{ op: "shape.set", args: { item_id: itemId, shape: "ellipse" } }],
		]);
		await expect(page.getByLabel("Corner radius", { exact: true })).toHaveCount(
			0,
		);
	});
});
