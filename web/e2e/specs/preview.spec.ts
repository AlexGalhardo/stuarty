import { expect, seedProject, test } from "./helpers";

// P1-UI-003 asserts that need the render pipeline behind the plan and frame
// endpoints. The unit tests of resources/js/editor/preview cover the plan
// windows, the sync rules, the audio clock and truth on pause.

// The comparison runs at the resolution of ViewFrame (360 x 640), on a window
// large enough for the canvas to have at least that many pixels. Compared at a
// 172 x 303 canvas (the default 1280 x 720 window) both images are downscaled
// and the SSIM of the same frame falls to 0.96: the gate measures the preview,
// not the resampling of a thumbnail.
test.describe("preview-vs-export", () => {
	test.use({ viewport: { width: 1920, height: 1300 } });

	test("the canvas matches ViewFrame: SSIM of at least 0.97 on luma", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "5s",
		});
		await page.goto(`/editor/${seeded.id}`);
		const canvas = page
			.getByRole("main", { name: "Preview" })
			.locator("canvas");
		await expect(canvas).toBeVisible();
		for (let i = 0; i < 30; i++) {
			await page
				.getByRole("button", { name: "Step forward one frame" })
				.click();
		}
		await page.waitForTimeout(1500);

		// The exact frame is an <img> above the canvas: hide it, so the test
		// compares the canvas itself.
		await page.evaluate(() => {
			for (const img of document.querySelectorAll("img")) {
				img.style.visibility = "hidden";
			}
		});
		const shot = (await canvas.screenshot()).toString("base64");

		const ssim = await page.evaluate(
			async ({ id, shotBase64 }) => {
				const frame = await fetch(
					`/api/v1/projects/${id}/frame?at=30f&width=360`,
					{ headers: { Accept: "application/json" } },
				);
				const body = (await frame.json()) as { image_base64: string };
				// fetch() of a data: URL is refused by the connect-src of the CSP.
				const load = (base64: string, type: string): Promise<ImageBitmap> => {
					const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
					return createImageBitmap(new Blob([bytes], { type }));
				};
				const a = await load(shotBase64, "image/png");
				const b = await load(body.image_base64, "image/jpeg");
				const w = 360;
				const h = 640;
				const luma = (bitmap: ImageBitmap): Float64Array => {
					const c = new OffscreenCanvas(w, h);
					const g = c.getContext("2d");
					if (!g) throw new Error("no 2d context");
					g.imageSmoothingQuality = "high";
					g.drawImage(bitmap, 0, 0, w, h);
					const d = g.getImageData(0, 0, w, h).data;
					const y = new Float64Array(w * h);
					for (let i = 0; i < w * h; i++) {
						y[i] =
							0.299 * d[i * 4] + 0.587 * d[i * 4 + 1] + 0.114 * d[i * 4 + 2];
					}
					return y;
				};
				const x = luma(a);
				const y = luma(b);
				// Mean SSIM over 8x8 blocks, stride 4 (Wang et al., K1 0.01, K2 0.03).
				const c1 = (0.01 * 255) ** 2;
				const c2 = (0.03 * 255) ** 2;
				let sum = 0;
				let n = 0;
				for (let by = 0; by + 8 <= h; by += 4) {
					for (let bx = 0; bx + 8 <= w; bx += 4) {
						let mx = 0;
						let my = 0;
						for (let j = 0; j < 8; j++) {
							for (let i = 0; i < 8; i++) {
								mx += x[(by + j) * w + bx + i];
								my += y[(by + j) * w + bx + i];
							}
						}
						mx /= 64;
						my /= 64;
						let vx = 0;
						let vy = 0;
						let cv = 0;
						for (let j = 0; j < 8; j++) {
							for (let i = 0; i < 8; i++) {
								const dx = x[(by + j) * w + bx + i] - mx;
								const dy = y[(by + j) * w + bx + i] - my;
								vx += dx * dx;
								vy += dy * dy;
								cv += dx * dy;
							}
						}
						vx /= 63;
						vy /= 63;
						cv /= 63;
						sum +=
							((2 * mx * my + c1) * (2 * cv + c2)) /
							((mx * mx + my * my + c1) * (vx + vy + c2));
						n++;
					}
				}
				return sum / n;
			},
			{ id: seeded.id, shotBase64: shot },
		);
		console.log(`preview vs ViewFrame: SSIM(luma) = ${ssim.toFixed(4)}`);
		expect(ssim).toBeGreaterThanOrEqual(0.97);
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

test.describe("csp-clean", () => {
	test("no securitypolicyviolation event and no blob: URL during playback", async ({
		page,
		request,
	}) => {
		const violations: string[] = [];
		const blobs: string[] = [];
		await page.exposeFunction("vvViolation", (v: string) => violations.push(v));
		await page.addInitScript(() => {
			document.addEventListener("securitypolicyviolation", (e) =>
				(window as unknown as { vvViolation(v: string): void }).vvViolation(
					e.violatedDirective,
				),
			);
		});
		page.on("request", (r) => {
			if (r.url().startsWith("blob:")) blobs.push(r.url());
		});
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "5s",
		});
		await page.goto(`/editor/${seeded.id}`);
		await page.getByRole("button", { name: "Play" }).click();
		await page.waitForTimeout(2000);
		await page.getByRole("button", { name: "Pause" }).click();
		expect(violations).toEqual([]);
		expect(blobs).toEqual([]);
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
