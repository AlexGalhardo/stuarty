import type { Page } from "@playwright/test";
import { expect, seedProject, test } from "./helpers";
import { openInspectorTab } from "./sections";

interface Operation {
	op: string;
	args: Record<string, unknown>;
}

function recordOps(page: Page): Operation[] {
	const ops: Operation[] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			const body = r.postDataJSON() as { operations: Operation[] };
			ops.push(...body.operations);
		}
	});
	return ops;
}

// TASK-0254, SDD-012 section 12: the voice filter, copy and paste attributes
// and the markers section. Each sends one command and draws what the core
// answers; the page computes nothing.
test.describe("inspector-gaps", () => {
	test("the voice filter select sends item.set_audio with the preset", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.locator("[data-item-id]").click();
		const ops = recordOps(page);

		await openInspectorTab(page, "Audio");
		const voice = page.getByLabel("Voice filter");
		await expect(voice).toHaveValue("none");
		await voice.selectOption({ label: "Warm" });
		await expect.poll(() => ops.length).toBe(1);
		expect(ops[0]).toEqual({
			op: "item.set_audio",
			args: { item_id: seeded.itemIds[0], voice: "warm" },
		});
		await expect(voice).toHaveValue("warm");

		await page.reload();
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await page.locator("[data-item-id]").click();
		await openInspectorTab(page, "Audio");
		await expect(page.getByLabel("Voice filter")).toHaveValue("warm");
	});

	test("Mod+Shift+C then Mod+Shift+V sends item.paste_attributes from the first item to the other", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}`);
		const items = page.locator("[data-item-id]");
		await expect(items).toHaveCount(2);
		const [first, second] = seeded.itemIds;

		await items.first().click();
		await page.keyboard.press("Control+Shift+c");
		await items.nth(1).click();
		const ops = recordOps(page);
		await page.keyboard.press("Control+Shift+v");
		await expect.poll(() => ops.length).toBe(1);
		expect(ops[0]?.op).toBe("item.paste_attributes");
		expect(ops[0]?.args).toMatchObject({
			from_item_id: first,
			item_ids: [second],
		});
		expect(ops[0]?.args.attributes).toEqual(
			expect.arrayContaining(["color", "audio", "transform"]),
		);
	});

	test("paste attributes does nothing before a copy", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		await page.locator("[data-item-id]").nth(1).click();
		const ops = recordOps(page);
		await page.keyboard.press("Control+Shift+v");
		await page.waitForTimeout(300);
		expect(ops).toEqual([]);
	});

	test("the markers section sets the label and the position and removes a marker", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		await expect(page.getByRole("heading", { name: "Project" })).toBeVisible();
		const ruler = page.getByRole("slider", { name: "Playhead" });
		await ruler.focus();
		await page.keyboard.press("PageUp");
		await page.keyboard.press("m");
		const markers = page.locator('[data-testid="ruler-marker"]');
		await expect(markers).toHaveCount(1);
		const frame = await markers.first().getAttribute("data-frame");
		const row = page.locator(`[data-marker-row="${frame}"]`);
		await expect(row).toBeVisible();
		const ops = recordOps(page);

		const label = row.getByLabel("Label");
		await label.fill("Hook");
		await label.press("Enter");
		await expect.poll(() => ops.length).toBe(1);
		expect(ops[0]).toEqual({
			op: "marker.set",
			args: { at: `${frame}f`, label: "Hook" },
		});
		await expect(row.getByLabel("Label")).toHaveValue("Hook");

		const position = row.getByLabel("Position");
		await position.fill("3");
		await position.press("Enter");
		await expect.poll(() => ops.length).toBe(2);
		expect(ops[1]).toMatchObject({
			op: "marker.set",
			args: { at: `${frame}f`, to: "3s" },
		});
		await expect(page.locator('[data-marker-row="90"]')).toBeVisible();
		await expect(markers).toHaveCount(1);

		await page
			.locator('[data-marker-row="90"]')
			.getByRole("button", { name: "Remove marker" })
			.click();
		await expect.poll(() => ops.length).toBe(3);
		expect(ops[2]).toEqual({ op: "marker.remove", args: { at: "90f" } });
		await expect(markers).toHaveCount(0);
		await expect(page.locator("[data-marker-row]")).toHaveCount(0);
	});
});
