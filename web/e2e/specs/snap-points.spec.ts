import { expect, seedProject, test } from "./helpers";

// P2-UI-008 (SDD-012 sections 7.7 and 7.8): a gesture asks the core for the snap
// points once per revision and snaps to the nearest of the playhead, frame 0 and
// the points; Alt suspends it. The probe of the end-to-end build runs what the
// first pointer move of a move or trim gesture runs.
test.describe("snap-points", () => {
	test("snaps to an item edge, the playhead and frame 0, Alt suspends, one request per revision", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		const requests: string[] = [];
		page.on("request", (r) => {
			if (r.method() === "GET" && r.url().includes("/snap-points")) {
				requests.push(r.url());
			}
		});
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await expect(page.locator("[data-item-id]")).toHaveCount(2);
		await page.waitForFunction(() => window.__vv?.snapProbe !== undefined);

		const starts = await page.evaluate(() => {
			const doc = window.__vv?.document as {
				items: Record<string, { start: number }>;
			};
			return Object.values(doc.items)
				.map((i) => i.start)
				.sort((a, b) => a - b);
		});
		const edge = starts[1];
		expect(edge).toBeGreaterThan(20);

		// The playhead goes to frame 10, away from both items edges.
		await page.getByRole("slider", { name: "Playhead" }).focus();
		for (let i = 0; i < 10; i++) await page.keyboard.press("ArrowRight");

		const probe = (frame: number, alt = false) =>
			page.evaluate(
				([f, a]) => window.__vv?.snapProbe?.({ frame: f, alt: a }),
				[frame, alt] as const,
			);

		expect(await probe(edge + 1)).toMatchObject({ frame: edge });
		await expect(page.getByTestId("snap-line")).toHaveAttribute(
			"data-frame",
			String(edge),
		);
		expect(await probe(12)).toMatchObject({ frame: 10, kind: "playhead" });
		expect(await probe(3)).toMatchObject({ frame: 0, kind: "timeline_start" });
		expect(await probe(edge + 1, true)).toMatchObject({
			frame: edge + 1,
			line: null,
		});
		await expect(page.getByTestId("snap-line")).toHaveCount(0);
		expect(requests).toHaveLength(1);

		// A new revision makes the next gesture fetch once more, and only once.
		const revision = await page.evaluate(() => window.__vv?.revision);
		await page.evaluate(
			(item) =>
				window.__vv?.dispatch?.([
					{ op: "item.rename", args: { item_id: item, name: "Renamed" } },
				]),
			seeded.itemIds[0],
		);
		await page.waitForFunction(
			(r) => (window.__vv?.revision ?? 0) > (r ?? 0),
			revision,
		);
		await probe(edge + 1);
		await probe(edge + 2);
		expect(requests).toHaveLength(2);
	});

	test("N toggles snapping and the Snap button shows it", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 2 });
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const snap = page.getByRole("button", { name: "Snap" });
		await expect(snap).toHaveAttribute("aria-pressed", "true");
		await page.locator("[data-item-id]").first().click();
		await page.keyboard.press("n");
		await expect(snap).toHaveAttribute("aria-pressed", "false");
		await snap.click();
		await expect(snap).toHaveAttribute("aria-pressed", "true");
	});
});
