import { lumaSsim } from "../support/ssim";
import {
	canvasOf,
	chunkIndex,
	MIN_SSIM,
	pixelsOfBoth,
	random,
	SEEKS,
	seekByRuler,
	skipWithoutWebCodecs,
} from "../support/webcodecs";
import { expect, seedProject, test } from "./helpers";

// P5-UI-002 (SDD-013 section 5.5, SDD-012 section 11.2 item 8): the preview
// decodes the VP9 proxy with WebCodecs in a worker. The canvas says what it
// drew: `data-drawn` the frame, `data-frame-us` the timestamp of the decoded
// sample, `data-peak-frames` the most VideoFrames that were open at once and
// `data-fallback-assets` the assets drawn by <video> instead.
//
// Chromium, Firefox and WebKit run it where the browser has WebCodecs and can
// decode the proxy's VP9; elsewhere the test skips and names the browser.

test.describe("webcodecs", () => {
	test("webcodecs-seek-is-frame-exact: 500 random seeks decode the sample the plan asks for", async ({
		page,
		request,
		browserName,
	}) => {
		test.setTimeout(400_000);
		await skipWithoutWebCodecs(page, (c, r) => test.skip(c, r), browserName);
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "10s",
		});
		const index = await chunkIndex(page, seeded.id, seeded.assetId);
		await page.goto(`/editor/${seeded.id}`);
		const canvas = canvasOf(page);
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 20_000,
		});
		expect(await canvas.getAttribute("data-fallback-assets")).toBe("0");

		// The ruler is longer than the 300 frames of the clip: map a fraction of it
		// to a frame once, so the seeks land on the clip.
		const low = await seekByRuler(page, 0.1);
		const high = await seekByRuler(page, 0.9);
		const perFraction = (high - low) / 0.8;
		const fractionOf = (frame: number): number =>
			0.1 + (frame - low) / perFraction;
		const clipFrames = index.timestamps_us.length;

		const next = random(20261116);
		const compared: number[] = [];
		for (let i = 0; i < SEEKS; i++) {
			const target = Math.floor(next() * (clipFrames - 2)) + 1;
			const frame = await seekByRuler(page, fractionOf(target));
			await expect(canvas).toHaveAttribute("data-drawn", String(frame), {
				timeout: 20_000,
			});
			// The clip starts at frame 0 of the source at normal speed: the plan asks
			// for the source second frame / 30, whose sample is number `frame`.
			const want = index.timestamps_us[Math.min(frame, clipFrames - 1)];
			expect(want, `sample ${frame} is in the index`).toBeDefined();
			expect(frame, "the seek landed on the clip").toBeLessThan(clipFrames);
			expect(
				Number(await canvas.getAttribute("data-frame-us")),
				`the sample decoded for frame ${frame}`,
			).toBe(want);
			if (i % 50 === 0) compared.push(frame);
		}
		expect(await canvas.getAttribute("data-fallback-assets")).toBe("0");

		// The pixels are the export's frame, in the tolerance of the preview
		// (SDD-013 section 5.3 item 7), not the neighbouring frame.
		for (const frame of compared) {
			await seekByRuler(page, fractionOf(frame));
			const shown = Number(
				await page
					.getByRole("slider", { name: "Playhead" })
					.getAttribute("aria-valuenow"),
			);
			await expect(canvas).toHaveAttribute("data-drawn", String(shown), {
				timeout: 20_000,
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
