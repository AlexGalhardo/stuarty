import { NO_VIDEO_DECODE_REASON, noVideoDecode } from "../support/browsers";
import { canvasOf } from "../support/webcodecs";
import { expect, seedProject, test } from "./helpers";

// Epic 26 task 5 (SDD-012 section 11.1): the executor is chosen by feature
// detection and a setting. A browser without a feature the renderer's worker needs
// still previews, with PixiJS, and says which feature is missing; a worker that
// cannot start (the module is refused) falls back the same way.
test.describe("preview-executor-fallback", () => {
	test("a browser without VideoDecoder previews with PixiJS and names the missing feature", async ({
		page,
		request,
		browserName,
	}) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON);
		const lines: string[] = [];
		page.on("console", (message) => {
			if (message.text().startsWith("preview:")) lines.push(message.text());
		});
		await page.addInitScript(() => {
			Reflect.deleteProperty(window, "VideoDecoder");
		});
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "5s",
		});
		await page.goto(`/editor/${seeded.id}?executor=rust`);
		const canvas = canvasOf(page);
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 30_000,
		});
		await expect(canvas).toHaveAttribute("data-executor", "pixi");
		await expect(canvas).toHaveAttribute(
			"data-executor-reason",
			"the browser lacks VideoDecoder",
		);
		expect(lines).toEqual(["preview: the browser lacks VideoDecoder"]);
	});

	test("a renderer module that does not load falls back to PixiJS", async ({
		page,
		request,
		browserName,
	}) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON);
		await page.route("**/*.wasm", (route) => route.abort());
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "5s",
		});
		await page.goto(`/editor/${seeded.id}?executor=rust`);
		const canvas = canvasOf(page);
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 30_000,
		});
		await expect(canvas).toHaveAttribute("data-executor", "pixi");
		expect(await canvas.getAttribute("data-executor-reason")).toContain(
			"the renderer worker did not start",
		);
	});

	test("the setting forces PixiJS, and the editor chooses by itself without one", async ({
		page,
		request,
		browserName,
	}) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON);
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "5s",
		});
		await page.goto(`/editor/${seeded.id}?executor=pixi`);
		const canvas = canvasOf(page);
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 30_000,
		});
		await expect(canvas).toHaveAttribute("data-executor", "pixi");
		await expect(canvas).toHaveAttribute(
			"data-executor-reason",
			"forced by the setting",
		);
	});
});
