import { nestedProject } from "../support/nested";
import { canvasOf, skipWithoutWebCodecs } from "../support/webcodecs";
import { expect, test } from "./helpers";

// Epic 52 (SDD-013 section 3.12 and 4.7.1): a composition with an opacity and a
// blend is one unit of its own. While the timeline plays, the preview draws it
// as a unit, with the renderer of the export in its worker, and asks the server
// for no picture of a frame: only the paused frame comes from the server.

const playhead = (page: import("@playwright/test").Page) =>
	page.getByRole("slider", { name: "Playhead" });

// The brightest channel in the canvas, 0 to 255.
async function brightest(
	page: import("@playwright/test").Page,
): Promise<number> {
	return canvasOf(page).evaluate((el) => {
		const canvas = el as HTMLCanvasElement;
		const g = canvas.getContext("2d");
		if (g === null) return -1;
		const data = g.getImageData(0, 0, canvas.width, canvas.height).data;
		let max = 0;
		for (let i = 0; i < data.length; i += 4) {
			max = Math.max(max, data[i] ?? 0, data[i + 1] ?? 0, data[i + 2] ?? 0);
		}
		return max;
	});
}

test.describe("preview-nested-live", () => {
	test("plays a composition with an opacity and a blend as one unit, with no frame request", async ({
		page,
		request,
		browserName,
	}) => {
		test.setTimeout(180_000);
		await skipWithoutWebCodecs(page, (c, r) => test.skip(c, r), browserName); // verify:env-skip no WebCodecs VP9 decoder in this browser
		const project = await nestedProject(request, { levels: 1 });

		const frameRequests: string[] = [];
		page.on("request", (r) => {
			if (new URL(r.url()).pathname.endsWith("/frame")) {
				frameRequests.push(r.url());
			}
		});

		await page.goto(`/editor/${project.id}?executor=rust`);
		const canvas = canvasOf(page);
		await expect(canvas).toHaveAttribute("data-executor", "rust");
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 60_000,
		});

		// A first frame is drawn at rest: the group is on the canvas at 50 %, so no
		// channel reaches the full white the clips have, and it is not empty (the
		// browser used to leave the group out).
		const resting = await brightest(page);
		expect(resting).toBeGreaterThan(20);
		expect(resting).toBeLessThanOrEqual(150);

		// The paused frame the server draws is a request of its own; count from
		// the moment playback starts.
		const before = frameRequests.length;
		await page.getByRole("button", { name: "Play" }).click();
		await expect(canvas).toHaveAttribute("data-clock", /audio|wall/);
		const presented = Number(await canvas.getAttribute("data-presented"));
		await expect
			.poll(
				async () => Number(await playhead(page).getAttribute("aria-valuenow")),
				{ timeout: 60_000 },
			)
			.toBeGreaterThan(30);
		// Frames reached the canvas while playing and each one holds the group.
		expect(Number(await canvas.getAttribute("data-presented"))).toBeGreaterThan(
			presented,
		);
		const playing = await brightest(page);
		expect(playing).toBeGreaterThan(20);
		expect(playing).toBeLessThanOrEqual(150);
		// No ViewFrame request while the timeline plays.
		expect(frameRequests.slice(before)).toEqual([]);

		await page.getByRole("button", { name: "Pause" }).click();
	});
});
