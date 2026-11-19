import { NO_VIDEO_DECODE_REASON, noVideoDecode } from "../support/browsers";
import { canvasOf } from "../support/webcodecs";
import { caller, expect, seedProject, solidClip, test } from "./helpers";

// VV-093: a green-screen clip with a chroma key over a background clip showed
// black behind the subject in the preview while the export showed the
// background. The key must reveal the layer below, on both executors, from the
// first frame the canvas marks as drawn.
for (const executor of ["rust", "pixi"] as const) {
	test(`vv-093: a keyed clip shows the background below it (${executor})`, async ({
		page,
		request,
		browserName,
	}) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON);
		const seeded = await seedProject(request, {
			// The first track is the bottom one: the blue background, then the green clip.
			files: [solidClip("blue"), solidClip("green")],
			tracks: 2,
			perTrack: 1,
			clipDuration: "1s",
		});
		const call = await caller(request);
		const head = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "clip.set_chroma_key",
					args: {
						item_id: seeded.itemIds[1],
						color: "#008000",
						similarity: 0.4,
						smoothness: 0.1,
						spill: 0,
					},
				},
			],
		});
		await page.goto(`/editor/${seeded.id}?executor=${executor}`);
		const canvas = canvasOf(page);
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 30_000,
		});
		const pixel = await canvas.evaluate((el) => {
			const source = el as HTMLCanvasElement;
			const probe = document.createElement("canvas");
			probe.width = source.width;
			probe.height = source.height;
			const context = probe.getContext("2d");
			if (context === null) return null;
			context.drawImage(source, 0, 0);
			return Array.from(
				context.getImageData(source.width >> 1, source.height >> 1, 1, 1).data,
			);
		});
		const [r = 255, g = 255, b = 0] = pixel ?? [];
		// Blue, not black and not the green that was keyed out.
		expect(b).toBeGreaterThan(150);
		expect(r).toBeLessThan(80);
		expect(g).toBeLessThan(80);
	});
}
