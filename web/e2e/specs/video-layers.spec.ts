import { execFileSync } from "node:child_process";
import { expect, seedProject, test } from "./helpers";

// SDD-012 section 11.2 item 5: MAX_VIDEO_LAYERS video elements decode at once.
// 4, 6 and 8 simultaneous video layers play for 10 seconds; the spec measures
// the frames the elements dropped, the JS heap and the memory of the renderer
// process of the tab (NFR-008, under 1 GB). It runs in the "perf" project with
// the other load-sensitive budgets (VUKEM_VIDEO_PERF=1): next to another spec
// the dropped frames measure the neighbour.
//
// The gate is the chosen cap of 4. Above it the preview decodes only the four
// topmost layers and the others show their thumbnail still, so 6 and 8 layers
// measure the same cap under a heavier plan; they are recorded, not gated on
// dropped frames, but the tab memory is gated at every count.
const MAX_DROPPED = 0.02;
const TAB_MEMORY_LIMIT_MB = 1024;

// The working set of a process in MB, from the operating system.
function processMb(pid: number): number {
	if (process.platform === "win32") {
		const out = execFileSync(
			"tasklist",
			["/FI", `PID eq ${pid}`, "/FO", "CSV", "/NH"],
			{ encoding: "utf8" },
		);
		const kb = Number(out.split('","')[4]?.replace(/[^0-9]/g, ""));
		return Number.isFinite(kb) ? kb / 1024 : 0;
	}
	const out = execFileSync("ps", ["-o", "rss=", "-p", String(pid)], {
		encoding: "utf8",
	});
	return Number(out.trim()) / 1024;
}

test.describe("video-layers-budget", () => {
	for (const layers of [4, 6, 8]) {
		test(`${layers} simultaneous video layers: dropped frames and tab memory over 10 s`, async ({
			page,
			request,
			browser,
		}) => {
			test.setTimeout(180_000);
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
			// The cap of MAX_VIDEO_LAYERS is that of pooled <video> elements.
			await page.goto(`/editor/${seeded.id}?decoder=video`);
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

			// The renderer processes of the browser: the tab is one of them.
			const cdp = await browser.newBrowserCDPSession();
			const info = (await cdp.send("SystemInfo.getProcessInfo")) as {
				processInfo: { type: string; id: number }[];
			};
			await cdp.detach();
			const renderers = info.processInfo
				.filter((p) => p.type === "renderer")
				.map((p) => processMb(p.id));
			const tabMb = Math.max(0, ...renderers);

			console.log(
				`video-layers ${layers}: ${quality.videos} video elements, ${quality.dropped} of ${quality.total} frames dropped (${((100 * quality.dropped) / Math.max(quality.total, 1)).toFixed(2)}%), JS heap ${quality.heapMb?.toFixed(0)} MB, renderer process ${tabMb.toFixed(0)} MB`,
			);
			expect(quality.total).toBeGreaterThan(0);
			expect(tabMb, "tab memory").toBeLessThan(TAB_MEMORY_LIMIT_MB);
			if (layers <= 4) {
				expect(quality.dropped / quality.total).toBeLessThan(MAX_DROPPED);
			}
		});
	}
});
