import { expect, seedProject, test } from "./helpers";

// VV-035: M puts a document marker at the playhead (marker.add) and M again
// removes it; the ruler draws it and Mod+Shift+Left and Right jump between
// markers. I and O draw the in and out span on the ruler.
test.describe("markers and in/out points", () => {
	test("M adds and removes a marker, the ruler draws it, the jump keys move the playhead", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const ops: string[] = [];
		page.on("request", (r) => {
			if (r.method() === "POST" && r.url().endsWith("/edit")) {
				const body = r.postDataJSON() as { operations: { op: string }[] };
				ops.push(...body.operations.map((o) => o.op));
			}
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const ruler = page.getByRole("slider", { name: "Playhead" });
		const markers = page.locator('[data-testid="ruler-marker"]');

		await ruler.focus();
		await page.keyboard.press("PageUp");
		await page.keyboard.press("m");
		await expect(markers).toHaveCount(1);
		expect(ops).toEqual(["marker.add"]);

		await page.keyboard.press("PageUp");
		await page.keyboard.press("m");
		await expect(markers).toHaveCount(2);

		await page.keyboard.press("Control+Shift+ArrowLeft");
		const first = Number(await markers.first().getAttribute("data-frame"));
		await expect(ruler).toHaveAttribute("aria-valuenow", String(first));
		await page.keyboard.press("m");
		await expect(markers).toHaveCount(1);
		expect(ops).toEqual(["marker.add", "marker.add", "marker.remove"]);

		await page.keyboard.press("i");
		await page.keyboard.press("PageUp");
		await page.keyboard.press("o");
		await expect(page.locator('[data-testid="ruler-range"]')).toBeVisible();
		await page.keyboard.press("Control+Shift+x");
		await expect(page.locator('[data-testid="ruler-range"]')).toHaveCount(0);
	});
});
