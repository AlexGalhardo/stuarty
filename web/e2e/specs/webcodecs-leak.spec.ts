import {
	canvasOf,
	OPEN_FRAMES_BOUND,
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
	test("webcodecs-no-leak: the open VideoFrames stay bounded over 500 seeks and a playback", async ({
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
		await page.goto(`/editor/${seeded.id}`);
		const canvas = canvasOf(page);
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 20_000,
		});
		const next = random(42);
		for (let i = 0; i < SEEKS; i++) {
			const frame = await seekByRuler(page, next());
			await expect(canvas).toHaveAttribute("data-drawn", String(frame), {
				timeout: 20_000,
			});
		}
		await page.getByRole("button", { name: "Go to start" }).click();
		await page.getByRole("button", { name: "Play" }).click();
		await page.waitForTimeout(3000);
		await page.getByRole("button", { name: "Pause" }).click();
		const open = Number(await canvas.getAttribute("data-open-frames"));
		const peak = Number(await canvas.getAttribute("data-peak-frames"));
		expect(peak).toBeGreaterThan(0);
		expect(peak).toBeLessThanOrEqual(OPEN_FRAMES_BOUND);
		expect(open).toBeLessThanOrEqual(OPEN_FRAMES_BOUND);
		expect(await canvas.getAttribute("data-fallback-assets")).toBe("0");
	});
});
