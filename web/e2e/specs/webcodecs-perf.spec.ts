import { expect, seedProject, test } from "./helpers";

// NFR-003 on the WebCodecs path (P5-UI-002, SDD-013 section 5.3): while
// scrubbing, a proxy frame appears within 100 ms of a seek. The time is that
// from the pointer going down on the ruler to the canvas saying it shows the
// frame (`data-drawn`), measured inside the page. It runs in the `perf`
// project with the other load-sensitive budgets (STUARTY_PERF=1), alone.
const SEEKS = 60;
const BUDGET_MS = 100;

test.describe("webcodecs-scrub-latency", () => {
	test("a decoded proxy frame appears within 100 ms of a seek at the 95th percentile (NFR-003)", async ({
		page,
		request,
		browserName,
	}) => {
		test.setTimeout(180_000);
		await page.goto("/up");
		const supported = await page.evaluate(
			() => typeof VideoDecoder !== "undefined",
		);
		test.skip(!supported, `${browserName} has no WebCodecs`);
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "10s",
		});
		await page.goto(`/editor/${seeded.id}`);
		const canvas = page
			.getByRole("main", { name: "Preview" })
			.locator("canvas");
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 20_000,
		});
		await page.evaluate(() => {
			const w = window as unknown as {
				vvStart: number;
				vvSeeks: number[];
			};
			w.vvStart = 0;
			w.vvSeeks = [];
			addEventListener(
				"pointerdown",
				() => {
					w.vvStart = performance.now();
				},
				true,
			);
			const target = document.querySelector("canvas[data-drawn]");
			if (target === null) throw new Error("no canvas");
			new MutationObserver(() => {
				if (w.vvStart > 0 && target.hasAttribute("data-drawn")) {
					w.vvSeeks.push(performance.now() - w.vvStart);
					w.vvStart = 0;
				}
			}).observe(target, {
				attributes: true,
				attributeFilter: ["data-drawn"],
			});
		});

		const ruler = page.getByRole("slider", { name: "Playhead" });
		const box = await ruler.boundingBox();
		if (!box) throw new Error("no ruler");
		for (let i = 0; i < SEEKS; i++) {
			await page.mouse.click(
				box.x + 40 + ((i * 53) % Math.max(box.width - 80, 100)),
				box.y + box.height / 2,
			);
			await page.waitForTimeout(300);
		}
		const seeks = await page.evaluate(
			() => (window as unknown as { vvSeeks: number[] }).vvSeeks,
		);
		const sorted = [...seeks].sort((a, b) => a - b);
		const at = (q: number): number =>
			sorted[Math.min(sorted.length - 1, Math.floor(sorted.length * q))] ?? -1;
		console.log(
			`NFR-003 WebCodecs seek to decoded frame: p50 ${at(0.5).toFixed(0)} ms, p95 ${at(0.95).toFixed(0)} ms (${sorted.length} seeks)`,
		);
		expect(sorted.length).toBeGreaterThan(SEEKS / 2);
		expect(at(0.95)).toBeLessThanOrEqual(BUDGET_MS);
	});
});
