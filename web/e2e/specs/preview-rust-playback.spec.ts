import {
	canvasOf,
	OPEN_FRAMES_BOUND,
	skipWithoutWebCodecs,
} from "../support/webcodecs";
import { expect, seedProject, test } from "./helpers";

// Epic 26 task 4 (SDD-013 section 5.3 item 4, SDD-012 section 11.2 item 4):
// with the renderer of the export forced, playback is driven by the audio clock,
// the frames it draws reach the canvas, the open VideoFrames stay bounded and
// the paused frame is the one the playhead shows.
const playhead = (page: import("@playwright/test").Page) =>
	page.getByRole("slider", { name: "Playhead" });

test.describe("preview-rust-playback", () => {
	test("plays on the audio clock, draws the frames and stops on the playhead frame", async ({
		page,
		request,
		browserName,
	}) => {
		test.setTimeout(120_000);
		await skipWithoutWebCodecs(page, (c, r) => test.skip(c, r), browserName); // verify:env-skip no WebCodecs VP9 decoder in this browser
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "10s",
		});
		await page.goto(`/editor/${seeded.id}?executor=rust`);
		const canvas = canvasOf(page);
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 30_000,
		});
		await expect(canvas).toHaveAttribute("data-executor", "rust");

		await page.getByRole("button", { name: "Play" }).click();
		await expect(canvas).toHaveAttribute("data-clock", "audio");
		// 45 frames at 30 fps is a second and a half of the clock.
		await expect
			.poll(
				async () => Number(await playhead(page).getAttribute("aria-valuenow")),
				{
					timeout: 30_000,
				},
			)
			.toBeGreaterThan(45);
		await page.getByRole("button", { name: "Pause" }).click();

		const stopped = Number(await playhead(page).getAttribute("aria-valuenow"));
		await expect(canvas).toHaveAttribute("data-drawn", String(stopped), {
			timeout: 30_000,
		});
		const presented = Number(await canvas.getAttribute("data-presented"));
		// The renderer drew a good share of the frames the clock went through; a
		// slower one would still show the right frame, so this only guards a
		// worker that draws nothing.
		expect(presented).toBeGreaterThan(10);
		// And a frame drawn whole is not drawn again on the next animation frame:
		// without a vsync the page gets about 250 of them a second, against the 30
		// frames of the clock. A frame whose layers are still arriving is drawn
		// again, hence the margin.
		expect(presented).toBeLessThanOrEqual(3 * stopped + 20);
		expect(
			Number(await canvas.getAttribute("data-peak-frames")),
		).toBeLessThanOrEqual(OPEN_FRAMES_BOUND);
		expect(
			Number(await canvas.getAttribute("data-open-frames")),
		).toBeLessThanOrEqual(OPEN_FRAMES_BOUND);
		expect(await canvas.getAttribute("data-fallback-assets")).toBe("0");
	});
});
