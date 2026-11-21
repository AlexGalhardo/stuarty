import { caller, expect, seedProject, test } from "./helpers";

// SDD-012 section 2.2 and 5.5: the place is in the URL, and the tabs of a
// browser share one event stream per project (scenarios 3, 2 and 4 with two
// tabs open).
test.describe("deep-link-reload", () => {
	test("the playhead and the selection survive a reload and a second tab", async ({
		page,
		context,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 3 });
		const item = seeded.itemIds[0];
		await page.goto(`/editor/${seeded.id}?t=45f&item=${item}`);

		const playhead = (p: typeof page) =>
			p.getByRole("slider", { name: "Playhead" });
		const selected = (p: typeof page) =>
			p.locator(`[data-item-id="${item}"][aria-selected="true"]`);
		await expect(playhead(page)).toHaveAttribute("aria-valuenow", "45");
		await expect(selected(page)).toHaveCount(1);

		await page.reload();
		await expect(playhead(page)).toHaveAttribute("aria-valuenow", "45");
		await expect(selected(page)).toHaveCount(1);

		const second = await context.newPage();
		await second.goto(page.url());
		await expect(playhead(second)).toHaveAttribute("aria-valuenow", "45");
		await expect(selected(second)).toHaveCount(1);
	});

	test("the URL follows the playhead when it rests and the selection at once", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("button", { name: "Step forward one frame" }).click();
		await page.getByRole("button", { name: "Step forward one frame" }).click();
		await expect(page).toHaveURL(/\?t=2f$/, { timeout: 5000 });
		await page.locator("[data-item-id]").first().click();
		await expect(page).toHaveURL(/\?t=2f&item=itm_/);
	});

	test("two tabs open one event stream, and both see an edit of another actor", async ({
		page,
		context,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const streams: string[] = [];
		context.on("request", (r) => {
			// Only the stream of the project: the bundle has a chunk named events-<hash>.js too.
			if (/\/api\/v1\/projects\/[^/]+\/events(\?|$)/.test(r.url()))
				streams.push(r.url());
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(1);
		const second = await context.newPage();
		await second.goto(`/editor/${seeded.id}`);
		await expect(second.locator("[data-item-id]")).toHaveCount(1);

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
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		await expect(second.locator("[data-item-id]")).toHaveCount(2);
		expect(streams).toHaveLength(1);
	});

	test("an unknown project answers 404", async ({ page }) => {
		const response = await page.goto("/editor/prj_0000000000?t=5f");
		expect(response?.status()).toBe(404);
	});
});
