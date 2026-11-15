import { caller, expect, seedProject, test } from "./helpers";

// SDD-012 section 5.4, the safety net: with ?debug=1&verify=1 the editor
// fetches the document after every diff it applies and compares it with the
// patched one. A diff that does not describe the whole change shows up here.
test.describe("diff-safety-net", () => {
	test("the document patched from events equals the fetched document", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		await page.goto(`/editor/${seeded.id}?debug=1&verify=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);

		await page.getByRole("button", { name: "Step forward one frame" }).click();
		await page.locator("[data-item-id]").click();
		await page.getByRole("button", { name: "Split" }).click();
		await expect(page.locator("[data-item-id]")).toHaveCount(2);

		const agent = await caller(request, "claude-code");
		const head = (await agent("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await agent("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "clip.add",
					args: {
						track_id: seeded.trackIds[0],
						asset_id: seeded.assetId,
						start: "5s",
						source_in: "0s",
						duration: "1s",
					},
				},
			],
		});
		await expect(page.locator("[data-item-id]")).toHaveCount(3);
		await page.waitForTimeout(500);
		expect(await page.evaluate(() => window.__vv?.mismatches)).toEqual([]);
	});
});
