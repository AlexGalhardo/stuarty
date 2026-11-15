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
