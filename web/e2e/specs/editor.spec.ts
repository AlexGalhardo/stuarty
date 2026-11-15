import { expect, seedProject, test } from "./helpers";

// P1-UI-001 and P1-UI-002 asserts that need the server: the route of
// /editor/{project_id} (P1-SRV-003) and the HTTP API and event stream
// (P1-SRV-005). The unit tests of resources/js/editor cover the same logic
// against the fixture server.
test.describe("editor-route", () => {
	test("an unknown project answers 404", async ({ page }) => {
		const response = await page.goto("/editor/prj_0000000000");
		expect(response?.status()).toBe(404);
	});

	test("reloading restores the same document", async ({ page, request }) => {
		const seeded = await seedProject(request, { tracks: 2, perTrack: 3 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(6);
		await page.reload();
		await expect(page.locator("[data-item-id]")).toHaveCount(6);
	});
});

test.describe("editor-live-edit", () => {
	test("an edit made through the CLI appears without reload and names the actor", async () => {
		// Open the editor on a project, run the CLI (`vukem-video track add ...`)
		// as a separate process, expect the track in #timeline within 200 ms and
		// the activity indicator to read "local:cli is editing".
		test.fixme(true, "needs the CLI binary of P1-CLI-001");
	});
});

test.describe("timeline-200-items-perf", () => {
	test("scrolls and zooms 200 items with frames under 16 ms at the 95th percentile", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		// 5 tracks of 40 one-second clips: the 200-item project of NFR-001.
		const seeded = await seedProject(request, { tracks: 5, perTrack: 40 });
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]").first()).toBeVisible();

		const stats = await page.evaluate(async () => {
			const scroller = document.querySelector<HTMLElement>(
				"#timeline .overflow-auto",
			);
			const zoomIn = document.querySelector<HTMLElement>(
				'[aria-label="Zoom in"]',
			);
			const zoomOut = document.querySelector<HTMLElement>(
				'[aria-label="Zoom out"]',
			);
			if (!scroller || !zoomIn || !zoomOut) throw new Error("no timeline");
			const nextFrame = (): Promise<void> =>
				new Promise((r) => requestAnimationFrame(() => r()));

			// The axis must be wider than the viewport, or a scroll changes nothing
			// and the browser paces the frames at the display rate.
			zoomIn.click();
			zoomIn.click();
			await nextFrame();

			const times: number[] = [];
			let last = performance.now();
			for (let i = 0; i < 180; i++) {
				if (i % 30 === 0) zoomIn.click();
				if (i % 30 === 15) zoomOut.click();
				const max = scroller.scrollWidth - scroller.clientWidth;
				scroller.scrollLeft = max > 0 ? (i * 37) % max : 0;
				await nextFrame();
				const now = performance.now();
				times.push(now - last);
				last = now;
			}
			times.sort((a, b) => a - b);
			return {
				p50: times[Math.floor(times.length * 0.5)],
				p95: times[Math.floor(times.length * 0.95)],
				max: times[times.length - 1],
			};
		});
		const { p50, p95, max } = stats;
		console.log(
			`timeline 200 items: frame p50 ${p50.toFixed(1)} ms, p95 ${p95.toFixed(1)} ms, max ${max.toFixed(1)} ms`,
		);
		expect(p95).toBeLessThan(16);
	});
});
