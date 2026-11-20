import { canvasOf, skipWithoutWebCodecs } from "../support/webcodecs";
import { expect, seedProject, test } from "./helpers";

// Epic 26 task 8: playback of the same project with each executor, measured by
// the animation frames the page gets while the preview plays (the browser runs
// without a vsync, see playwright.config.ts, so an interval is the work of a
// frame). The numbers are logged, not asserted: budgets belong to epic 42, and
// a measurement is valid only on an idle machine, so the file is named for the
// "perf" project of playwright.config.ts (STUARTY_PERF=1, alone, after the other
// specs) and stays out of the browser matrix.
const DURATION_MS = 4000;

const playhead = (page: import("@playwright/test").Page) =>
	page.getByRole("slider", { name: "Playhead" });

for (const executor of ["pixi", "rust"] as const) {
	test(`preview-playback-measure: ${executor}`, async ({
		page,
		request,
		browserName,
	}) => {
		test.setTimeout(180_000);
		await skipWithoutWebCodecs(page, (c, r) => test.skip(c, r), browserName); // verify:env-skip no WebCodecs VP9 decoder in this browser
		const seeded = await seedProject(request, {
			tracks: 2,
			perTrack: 1,
			clipDuration: "10s",
		});
		await page.goto(`/editor/${seeded.id}?executor=${executor}`);
		const canvas = canvasOf(page);
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 30_000,
		});
		await expect(canvas).toHaveAttribute("data-executor", executor);
		await page.getByRole("button", { name: "Play" }).click();
		const before = Number((await canvas.getAttribute("data-presented")) ?? 0);
		const from = Number(await playhead(page).getAttribute("aria-valuenow"));
		const stats = await page.evaluate(async (ms) => {
			// Main-thread tasks of more than 50 ms while the preview plays.
			const longTasks: number[] = [];
			const observer = new PerformanceObserver((list) => {
				for (const entry of list.getEntries()) longTasks.push(entry.duration);
			});
			observer.observe({ entryTypes: ["longtask"] });
			const start = performance.now();
			let rafs = 0;
			while (performance.now() - start < ms) {
				await new Promise<void>((resolve) =>
					requestAnimationFrame(() => resolve()),
				);
				rafs++;
			}
			const elapsed = performance.now() - start;
			observer.disconnect();
			return {
				elapsed,
				rafs,
				longTasks: longTasks.length,
				blocked: longTasks.reduce((a, b) => a + b, 0),
			};
		}, DURATION_MS);
		const presented =
			Number((await canvas.getAttribute("data-presented")) ?? 0) - before;
		const advanced =
			Number(await playhead(page).getAttribute("aria-valuenow")) - from;
		await page.getByRole("button", { name: "Pause" }).click();
		// The worker's count is written every 10 frames while playing, so the frames
		// the renderer did not draw are known to within 10.
		const worker =
			executor === "rust"
				? `${presented} (${((presented * 1000) / stats.elapsed).toFixed(1)} per second, about ${Math.max(advanced - presented, 0)} of the clock's frames not drawn)`
				: "n/a (PixiJS draws on every animation frame)";
		console.log(
			`preview-playback-measure ${executor}: ${(stats.elapsed / 1000).toFixed(2)} s, the clock went through ${advanced} frames, ${stats.rafs} animation frames (${((stats.rafs * 1000) / stats.elapsed).toFixed(1)} per second), frames drawn by the renderer's worker ${worker}, main-thread tasks over 50 ms ${stats.longTasks} (${stats.blocked.toFixed(0)} ms)`,
		);
		expect(stats.rafs).toBeGreaterThan(0);
	});
}
