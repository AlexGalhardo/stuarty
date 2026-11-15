import { caller, expect, markerClip, seedProject, test } from "./helpers";

// P1-UI-003 asserts that need the render pipeline behind the plan and frame
// endpoints. The unit tests of resources/js/editor/preview cover the plan
// windows, the sync rules, the audio clock and truth on pause.

// ViewFrame is requested at the width of the canvas and compared at the size of
// the canvas, so no image is resampled: the gate measures the preview. Compared
// after downscaling both to a thumbnail the SSIM of the same frame falls to 0.96.
//
// The three Prototype cases of SDD-012 section 20.3: a clip with cover fit, the
// same clip with contain fit, and a clip with one caption. The clip is
// landscape, so cover and contain differ. Red and blue boxes in the clip are the
// markers of the displacement check: their centroids in the canvas and in
// ViewFrame must be at most 1 pixel apart at the preview size.
const SCENES: {
	name: string;
	fit: "cover" | "contain";
	caption?: string;
}[] = [
	{ name: "cover", fit: "cover" },
	{ name: "contain", fit: "contain" },
	{ name: "caption", fit: "cover", caption: "Meet the new editor" },
];

// The width of a PNG, from its IHDR.
function PNG_WIDTH(png: Buffer): number {
	return png.readUInt32BE(16);
}

test.describe("preview-vs-export", () => {
	test.use({ viewport: { width: 1920, height: 1300 } });

	for (const scene of SCENES) {
		test(`${scene.name}: SSIM of at least 0.97 on luma and markers within 1 pixel`, async ({
			page,
			request,
		}) => {
			const seeded = await seedProject(request, {
				tracks: 1,
				perTrack: 1,
				clipDuration: "5s",
				files: [markerClip(640, 360)],
			});
			const call = await caller(request);
			const head = (await call("GET", `/projects/${seeded.id}`)) as {
				revision: number;
			};
			const operations: unknown[] = [
				{
					op: "clip.set_framing",
					args: { item_id: seeded.itemIds[0], fit: scene.fit },
				},
			];
			if (scene.caption) {
				// A caption needs its own track: it would overlap the clip on theirs.
				operations.push(
					{
						op: "track.add",
						args: { kind: "visual", name: "Captions", id: "trk_c4p710n2m3" },
					},
					{
						op: "caption.add",
						args: {
							text: scene.caption,
							start: "0s",
							duration: "4s",
							track_id: "trk_c4p710n2m3",
						},
					},
				);
			}
			await call("POST", `/projects/${seeded.id}/edit`, {
				base_revision: head.revision,
				operations,
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
			await page.waitForTimeout(2000);

			// The exact frame is an <img> above the canvas: hide it, so the test
			// compares the canvas itself.
			await page.evaluate(() => {
				for (const img of document.querySelectorAll("img")) {
					img.style.visibility = "hidden";
				}
			});
			const shotBuffer = await canvas.screenshot();
			const shot = shotBuffer.toString("base64");

			const result = await page.evaluate(
				async ({ id, shotBase64, width }) => {
					const frame = await fetch(
						`/api/v1/projects/${id}/frame?at=30f&width=${width}`,
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
					// No resampling: the frame has the pixels of the canvas.
					const w = a.width;
					const h = a.height;
					const pixels = (bitmap: ImageBitmap): Uint8ClampedArray => {
						const c = new OffscreenCanvas(w, h);
						const g = c.getContext("2d");
						if (!g) throw new Error("no 2d context");
						g.imageSmoothingQuality = "high";
						g.drawImage(bitmap, 0, 0, w, h);
						return g.getImageData(0, 0, w, h).data;
					};
					const pa = pixels(a);
					const pb = pixels(b);
					const luma = (d: Uint8ClampedArray): Float64Array => {
						const y = new Float64Array(w * h);
						for (let i = 0; i < w * h; i++) {
							y[i] =
								0.299 * d[i * 4] + 0.587 * d[i * 4 + 1] + 0.114 * d[i * 4 + 2];
						}
						return y;
					};
					const x = luma(pa);
					const y = luma(pb);
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
					// The centroid of the pixels of a marker colour.
					const centroid = (
						d: Uint8ClampedArray,
						hit: (r: number, g: number, b: number) => boolean,
					): { x: number; y: number; count: number } => {
						let sx = 0;
						let sy = 0;
						let count = 0;
						for (let i = 0; i < w * h; i++) {
							if (hit(d[i * 4], d[i * 4 + 1], d[i * 4 + 2])) {
								sx += i % w;
								sy += Math.floor(i / w);
								count++;
							}
						}
						return { x: sx / count, y: sy / count, count };
					};
					const red = (r: number, g: number, b: number): boolean =>
						r > 180 && g < 80 && b < 80;
					const blue = (r: number, g: number, b: number): boolean =>
						b > 180 && r < 80 && g < 80;
					return {
						ssim: sum / n,
						canvas: { red: centroid(pa, red), blue: centroid(pa, blue) },
						frame: { red: centroid(pb, red), blue: centroid(pb, blue) },
					};
				},
				{ id: seeded.id, shotBase64: shot, width: PNG_WIDTH(shotBuffer) },
			);
			const shift = (
				p: { x: number; y: number },
				q: { x: number; y: number },
			): number => Math.hypot(p.x - q.x, p.y - q.y);
			console.log(
				`DBG ${scene.name} red d=(${(result.canvas.red.x - result.frame.red.x).toFixed(2)},${(result.canvas.red.y - result.frame.red.y).toFixed(2)}) blue d=(${(result.canvas.blue.x - result.frame.blue.x).toFixed(2)},${(result.canvas.blue.y - result.frame.blue.y).toFixed(2)})`,
			);
			const redShift = shift(result.canvas.red, result.frame.red);
			const blueShift = shift(result.canvas.blue, result.frame.blue);
			console.log(
				`preview-vs-export ${scene.name}: SSIM(luma) ${result.ssim.toFixed(4)}, red marker ${redShift.toFixed(2)} px (${result.canvas.red.count} / ${result.frame.red.count} px), blue marker ${blueShift.toFixed(2)} px (${result.canvas.blue.count} / ${result.frame.blue.count} px)`,
			);
			expect(result.ssim).toBeGreaterThanOrEqual(0.97);
			for (const [name, count, shifted] of [
				["red", result.frame.red.count, redShift],
				["blue", result.frame.blue.count, blueShift],
			] as const) {
				expect(count, `${name} marker visible in ViewFrame`).toBeGreaterThan(
					20,
				);
				expect(shifted, `${name} marker displacement`).toBeLessThanOrEqual(1);
			}
		});
	}
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
