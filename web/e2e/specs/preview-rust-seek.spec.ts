import { lumaSsim } from "../support/ssim";
import {
	canvasOf,
	chunkIndex,
	MIN_SSIM,
	OPEN_FRAMES_BOUND,
	pixelsOfBoth,
	random,
	seekByRuler,
	skipWithoutWebCodecs,
} from "../support/webcodecs";
import { expect, seedProject, test } from "./helpers";

// Epic 26 task 3 (SDD-013 sections 4.7 and 5.3): with the renderer of the export
// forced (`?executor=rust`), the proxy is decoded with WebCodecs through the
// chunk index, the frame is drawn by the renderer in its worker, and a seek lands
// on the sample the plan asks for. The canvas says what it drew: `data-executor`
// the executor, `data-drawn` the frame, `data-frame-us` the timestamp of the
// decoded sample and `data-peak-frames` the most VideoFrames open at once.
const SEEKS = 60;

test.describe("preview-rust-seek", () => {
	test("seeks decode the sample the plan asks for and the renderer draws the export's frame", async ({
		page,
		request,
		browserName,
	}) => {
		test.setTimeout(240_000);
		await skipWithoutWebCodecs(page, (c, r) => test.skip(c, r), browserName); // verify:env-skip no WebCodecs VP9 decoder in this browser
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "10s",
		});
		const index = await chunkIndex(page, seeded.id, seeded.assetId);
		await page.goto(`/editor/${seeded.id}?executor=rust`);
		const canvas = canvasOf(page);
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 30_000,
		});
		await expect(canvas).toHaveAttribute("data-executor", "rust");
		expect(await canvas.getAttribute("data-fallback-assets")).toBe("0");

		const low = await seekByRuler(page, 0.1);
		const high = await seekByRuler(page, 0.9);
		const perFraction = (high - low) / 0.8;
		const fractionOf = (frame: number): number =>
			0.1 + (frame - low) / perFraction;
		const clipFrames = index.timestamps_us.length;

		const next = random(20261119);
		const compared: number[] = [];
		for (let i = 0; i < SEEKS; i++) {
			const target = Math.floor(next() * (clipFrames - 2)) + 1;
			const frame = await seekByRuler(page, fractionOf(target));
			await expect(canvas).toHaveAttribute("data-drawn", String(frame), {
				timeout: 30_000,
			});
			const want = index.timestamps_us[Math.min(frame, clipFrames - 1)];
			expect(want, `sample ${frame} is in the index`).toBeDefined();
			expect(
				Number(await canvas.getAttribute("data-frame-us")),
				`the sample decoded for frame ${frame}`,
			).toBe(want);
			if (i % 10 === 0) compared.push(frame);
		}
		expect(await canvas.getAttribute("data-fallback-assets")).toBe("0");
		expect(
			Number(await canvas.getAttribute("data-peak-frames")),
			"open VideoFrames stay inside the cache bound",
		).toBeLessThanOrEqual(OPEN_FRAMES_BOUND);

		for (const frame of compared) {
			await seekByRuler(page, fractionOf(frame));
			const shown = Number(
				await page
					.getByRole("slider", { name: "Playhead" })
					.getAttribute("aria-valuenow"),
			);
			await expect(canvas).toHaveAttribute("data-drawn", String(shown), {
				timeout: 30_000,
			});
			const { canvas: a, exported: b } = await pixelsOfBoth(
				page,
				seeded.id,
				shown,
			);
			expect(lumaSsim(a, b), `SSIM at frame ${shown}`).toBeGreaterThanOrEqual(
				MIN_SSIM,
			);
		}
	});
});
