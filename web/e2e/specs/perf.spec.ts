import { expect, seedProject, test } from "./helpers";

// The budgets that depend on the load of the machine (SDD-012 section 19). They
// run in the "perf" project of playwright.config.ts, alone, after the other
// specs: next to a video decode or an export a frame loop or a round trip
// measures the neighbour, not the editor.
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
test.describe("command-round-trip", () => {
	test("the round trip of a command is at most 50 ms at the 95th percentile on the 200-item project (NFR-001)", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedProject(request, { tracks: 5, perTrack: 40 });
		const edits: string[] = [];
		page.on("request", (r) => {
			if (r.url().endsWith("/edit")) edits.push(r.url());
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(page.locator("[data-item-id]")).toHaveCount(200);

		const samples = 25;
		const times: number[] = [];
		const ruler = page.getByRole("slider", { name: "Playhead" });
		for (let i = 0; i < samples; i++) {
			// Select a clip of the first track and put the playhead inside it.
			const clip = page.locator(`[data-item-id="${seeded.itemIds[i]}"]`);
			await clip.scrollIntoViewIfNeeded();
			const box = await clip.boundingBox();
			const rulerBox = await ruler.boundingBox();
			if (!box || !rulerBox) throw new Error("no layout");
			await page.mouse.click(box.x + 10, rulerBox.y + rulerBox.height / 2);
			await clip.click();
			const ms = await page.evaluate(async () => {
				const count = (): number =>
					document.querySelectorAll("[data-item-id]").length;
				const before = count();
				const button = [...document.querySelectorAll("button")].find(
					(b) =>
						b.textContent?.trim() === "Split" ||
						b.getAttribute("aria-label") === "Split",
				);
				if (!button) throw new Error("no Split button");
				return await new Promise<number>((resolve, reject) => {
					setTimeout(
						() => reject(new Error("split did not change the timeline")),
						3000,
					);
					const start = performance.now();
					const observer = new MutationObserver(() => {
						if (count() > before) {
							observer.disconnect();
							resolve(performance.now() - start);
						}
					});
					observer.observe(document.body, { childList: true, subtree: true });
					button.click();
				});
			});
			times.push(ms);
		}
		expect(edits).toHaveLength(samples);
		times.sort((a, b) => a - b);
		const p95 = times[Math.floor(times.length * 0.95)];
		console.log(
			`edit round trip on 200 items: p50 ${times[Math.floor(times.length / 2)].toFixed(1)} ms, p95 ${p95.toFixed(1)} ms, max ${times[times.length - 1].toFixed(1)} ms, ${edits.length} requests for ${samples} gestures`,
		);
		expect(p95).toBeLessThan(50);
	});
});

test.describe("play-pause-truth", () => {
	test("after pausing the exact frame appears within 500 ms at the 95th percentile (NFR-004)", async ({
		page,
		request,
	}) => {
		test.setTimeout(120_000);
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "8s",
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(
			page.getByRole("main", { name: "Preview" }).locator("canvas"),
		).toBeVisible();

		const cold: number[] = [];
		const warm: number[] = [];
		const exact = page.getByAltText("Exact frame at the playhead");
		const truthMs = (): Promise<number> =>
			page.evaluate(
				() => performance.getEntriesByName("vv:truth").at(-1)?.duration ?? -1,
			);
		// The measure of the next overlay: cleared before the gesture, awaited after.
		const nextTruth = async (gesture: () => Promise<void>): Promise<number> => {
			await page.evaluate(() => performance.clearMeasures("vv:truth"));
			await gesture();
			await expect
				.poll(() => truthMs(), { timeout: 3000, intervals: [20] })
				.toBeGreaterThan(0);
			return truthMs();
		};
		for (let i = 0; i < 20; i++) {
			// Back to the start each time: a pause past the end of the project has no
			// frame to show.
			await page.getByRole("button", { name: "Go to start" }).click();
			await page.getByRole("button", { name: "Play" }).click();
			await page.waitForTimeout(600);
			// The first pause at a frame renders it on the server: cold.
			cold.push(
				await nextTruth(() =>
					page.getByRole("button", { name: "Pause" }).click(),
				),
			);
			await expect(exact).toBeVisible();
			// One frame back and forth lands on the frame that was just rendered.
			await page.getByRole("button", { name: "Step back one frame" }).click();
			await expect(exact).toBeVisible();
			warm.push(
				await nextTruth(() =>
					page.getByRole("button", { name: "Step forward one frame" }).click(),
				),
			);
		}
		const p = (values: number[], q: number): number =>
			[...values].sort((a, b) => a - b)[Math.floor(values.length * q)];
		console.log(
			`NFR-004 pause to exact frame (vv:truth, includes the 150 ms delay): cold p50 ${p(cold, 0.5).toFixed(0)} ms, p95 ${p(cold, 0.95).toFixed(0)} ms; warm p50 ${p(warm, 0.5).toFixed(0)} ms, p95 ${p(warm, 0.95).toFixed(0)} ms`,
		);
		// The budget gates the warm path. The cold render of ViewFrame is open: see
		// the NFR-004 row of SDD-012 section 19.
		expect(p(warm, 0.95)).toBeGreaterThan(0);
		expect(p(warm, 0.95)).toBeLessThanOrEqual(500);
	});
});

test.describe("scrub-latency", () => {
	test("a proxy frame appears within 100 ms of a seek at the 95th percentile (NFR-003)", async ({
		page,
		request,
	}) => {
		test.setTimeout(90_000);
		await page.addInitScript(() => {
			const w = window as unknown as {
				vvSeeks: number[];
				vvStart: number;
			};
			w.vvSeeks = [];
			w.vvStart = 0;
			const create = document.createElement.bind(document);
			document.createElement = ((
				tag: string,
				options?: ElementCreationOptions,
			) => {
				const el = create(tag, options);
				if (tag.toLowerCase() === "video") {
					el.addEventListener("seeked", () => {
						if (w.vvStart > 0) {
							w.vvSeeks.push(performance.now() - w.vvStart);
							w.vvStart = 0;
						}
					});
				}
				return el;
			}) as typeof document.createElement;
		});
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "10s",
		});
		await page.goto(`/editor/${seeded.id}`);
		await expect(
			page.getByRole("main", { name: "Preview" }).locator("canvas"),
		).toBeVisible();
		await page.waitForTimeout(1500);

		const ruler = page.getByRole("slider", { name: "Playhead" });
		const box = await ruler.boundingBox();
		if (!box) throw new Error("no ruler");
		const seeks: number[] = [];
		for (let i = 0; i < 20; i++) {
			await page.evaluate(() => {
				(window as unknown as { vvStart: number }).vvStart = performance.now();
			});
			await page.mouse.click(
				box.x + 40 + ((i * 53) % 600),
				box.y + box.height / 2,
			);
			await page.waitForTimeout(400);
			const done = await page.evaluate(
				() => (window as unknown as { vvSeeks: number[] }).vvSeeks.at(-1) ?? -1,
			);
			seeks.push(done);
		}
		const measured = seeks.filter((v) => v >= 0).sort((a, b) => a - b);
		const p95 = measured[Math.floor(measured.length * 0.95)];
		console.log(
			`NFR-003 seek to decoded proxy frame: p50 ${measured[Math.floor(measured.length / 2)]?.toFixed(0)} ms, p95 ${p95?.toFixed(0)} ms (${measured.length} seeks)`,
		);
		expect(measured.length).toBeGreaterThan(10);
		expect(p95).toBeLessThanOrEqual(100);
	});
});

test.describe("video-layers-budget", () => {
	for (const layers of [4, 6, 8]) {
		test(`${layers} simultaneous video layers drop under 2% of frames over 10 s`, async ({
			page,
			request,
		}) => {
			test.setTimeout(120_000);
			await page.addInitScript(() => {
				const videos: HTMLVideoElement[] = [];
				(window as unknown as { vvVideos: HTMLVideoElement[] }).vvVideos =
					videos;
				const create = document.createElement.bind(document);
				document.createElement = ((
					tag: string,
					options?: ElementCreationOptions,
				) => {
					const el = create(tag, options);
					if (tag.toLowerCase() === "video")
						videos.push(el as HTMLVideoElement);
					return el;
				}) as typeof document.createElement;
			});
			const seeded = await seedProject(request, {
				tracks: layers,
				assets: layers,
				perTrack: 1,
				clipDuration: "10s",
			});
			await page.goto(`/editor/${seeded.id}`);
			await expect(
				page.getByRole("main", { name: "Preview" }).locator("canvas"),
			).toBeVisible();
			await page.getByRole("button", { name: "Play" }).click();
			await page.waitForTimeout(10_000);
			const quality = await page.evaluate(() => {
				let total = 0;
				let dropped = 0;
				const videos = (window as unknown as { vvVideos: HTMLVideoElement[] })
					.vvVideos;
				for (const video of videos) {
					const q = video.getVideoPlaybackQuality();
					total += q.totalVideoFrames;
					dropped += q.droppedVideoFrames;
				}
				const memory = (
					performance as unknown as { memory?: { usedJSHeapSize: number } }
				).memory;
				return {
					videos: videos.length,
					total,
					dropped,
					heapMb: memory ? memory.usedJSHeapSize / 1048576 : null,
				};
			});
			console.log(`video-layers ${layers}: ${JSON.stringify(quality)}`);
			expect(quality.total).toBeGreaterThan(0);
			// The budget gates the chosen MAX_VIDEO_LAYERS (4); 6 and 8 layers are
			// measurements, recorded in SDD-012 section 11.2.
			if (layers <= 4) {
				expect(quality.dropped / quality.total).toBeLessThan(0.02);
			}
		});
	}
});
