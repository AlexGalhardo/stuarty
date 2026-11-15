import {
	audioClip,
	caller,
	expect,
	seedProject,
	test,
	testClip,
} from "./helpers";

// P2-UI-022 (SDD-012 section 7.4): a clip shows the thumbnail strip of its
// asset, an audio item draws its waveform on a canvas, and neither goes through
// a blob: URL.
test.describe("item-rendering", () => {
	test("a 5 s clip shows its strip and an audio item draws a non-empty waveform, with no blob: request", async ({
		page,
		request,
	}) => {
		const blobs: string[] = [];
		const stripRequests: string[] = [];
		page.on("request", (r) => {
			if (r.url().startsWith("blob:")) blobs.push(r.url());
			if (r.url().endsWith("/strip")) stripRequests.push(r.url());
		});

		const seeded = await seedProject(request, {
			files: [testClip(0), audioClip(0)],
			tracks: 1,
			perTrack: 1,
			clipDuration: "5s",
			audioTracks: 1,
		});
		const call = await caller(request);
		const head = (await call(
			"GET",
			`/projects/${seeded.id}?detail=document`,
		)) as { revision: number };
		await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "audio.add",
					args: {
						track_id: seeded.audioTrackIds[0],
						asset_id: seeded.assetIds[1],
						start: "0s",
					},
				},
			],
		});

		await page.goto(`/editor/${seeded.id}`);
		const clip = page.locator(`[data-item-id="${seeded.itemIds[0]}"]`);
		await expect(clip.locator("[data-strip]")).toHaveCount(1);
		await expect(clip.locator("[data-strip-tile]")).toHaveCount(5);
		const image = await clip
			.locator("[data-strip-tile]")
			.first()
			.evaluate((el) => getComputedStyle(el).backgroundImage);
		expect(image).toContain(`/media/${seeded.assetIds[0]}/strip`);
		expect(stripRequests.length).toBeGreaterThan(0);

		const waveform = page.locator("[data-waveform] canvas[data-drawn]");
		await expect(waveform).toHaveCount(1);
		await expect
			.poll(() =>
				waveform.evaluate((canvas: HTMLCanvasElement) => {
					const context = canvas.getContext("2d");
					if (context === null) return 0;
					const { data } = context.getImageData(
						0,
						0,
						canvas.width,
						canvas.height,
					);
					let columns = 0;
					for (let x = 0; x < canvas.width; x++) {
						for (let y = 0; y < canvas.height; y++) {
							if (data[(y * canvas.width + x) * 4 + 3] !== 0) {
								columns++;
								break;
							}
						}
					}
					return columns;
				}),
			)
			.toBeGreaterThan(0);
		expect(blobs).toEqual([]);
	});
});
