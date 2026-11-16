import { NO_VIDEO_DECODE_REASON, noVideoDecode } from "../support/browsers";
import { canvasOf, skipWithoutWebCodecs } from "../support/webcodecs";
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
	test("webcodecs-fallback: an asset without an index draws through <video>, the other through WebCodecs", async ({
		page,
		request,
		context,
		browserName,
	}) => {
		test.setTimeout(120_000);
		await skipWithoutWebCodecs(page, (c, r) => test.skip(c, r), browserName);
		await page.addInitScript(() => {
			const videos: string[] = [];
			(window as unknown as { vvVideoSrc: string[] }).vvVideoSrc = videos;
			const create = document.createElement.bind(document);
			document.createElement = ((
				tag: string,
				options?: ElementCreationOptions,
			) => {
				const el = create(tag, options);
				if (tag.toLowerCase() === "video") {
					const proto = Object.getOwnPropertyDescriptor(
						HTMLMediaElement.prototype,
						"src",
					);
					Object.defineProperty(el, "src", {
						set(v: string) {
							videos.push(v);
							proto?.set?.call(el, v);
						},
						get() {
							return proto?.get?.call(el);
						},
					});
				}
				return el;
			}) as typeof document.createElement;
		});
		const seeded = await seedProject(request, {
			assets: 2,
			tracks: 2,
			perTrack: 1,
			clipDuration: "3s",
		});
		const [withoutIndex, withIndex] = seeded.assetIds;
		if (!withoutIndex || !withIndex) throw new Error("two assets expected");
		// The same answer the route gives for an asset ingested before the index
		// existed (SDD-013 section 6.4).
		await context.route(`**/media/${withoutIndex}/chunks`, (route) =>
			route.fulfill({
				status: 404,
				contentType: "application/json",
				body: '{"error":{"code":"E_NOT_FOUND"}}',
			}),
		);
		await page.goto(`/editor/${seeded.id}`);
		const canvas = canvasOf(page);
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 20_000,
		});
		await expect(canvas).toHaveAttribute("data-fallback-assets", "1");
		const srcs = await page.evaluate(
			() => (window as unknown as { vvVideoSrc: string[] }).vvVideoSrc,
		);
		expect(srcs.some((s) => s.includes(withoutIndex))).toBe(true);
		expect(srcs.some((s) => s.includes(withIndex))).toBe(false);
		expect(
			Number(await canvas.getAttribute("data-peak-frames")),
		).toBeGreaterThan(0);
	});

	test("webcodecs-forced-video: ?decoder=video starts no worker and draws through <video>", async ({
		page,
		request,
		browserName,
	}) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON);
		let workers = 0;
		page.on("worker", () => {
			workers++;
		});
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "3s",
		});
		await page.goto(`/editor/${seeded.id}?decoder=video`);
		const canvas = canvasOf(page);
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 20_000,
		});
		expect(workers).toBe(0);
		expect(await canvas.getAttribute("data-peak-frames")).toBe("0");
	});
});
