import { NO_VIDEO_DECODE_REASON, noVideoDecode } from "../support/browsers";
import { alphaClip, expect, seedProject, solidClip, test } from "./helpers";

// VV-013, SDD-012 section 11.2: an asset with alpha has a WebM proxy and no
// chunk index, and the preview plays it through the <video> element and draws
// its alpha over the layers below.
test.describe("alpha-overlay", () => {
	test("a transparent half of an overlay shows the layer below", async ({
		page,
		request,
		browserName,
	}) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON);
		const failures: string[] = [];
		page.on("response", (r) => {
			if (r.status() >= 400 && !r.url().endsWith("/chunks")) {
				failures.push(`${r.status()} ${r.url()}`);
			}
		});
		const seeded = await seedProject(request, {
			files: [alphaClip(), solidClip("green")],
			tracks: 2,
			perTrack: 1,
			clipDuration: "1s",
		});
		await page.goto(`/editor/${seeded.id}`);
		const canvas = page
			.getByRole("main", { name: "Preview" })
			.locator("canvas");
		await expect(canvas).toHaveAttribute("data-drawn", "0", {
			timeout: 20_000,
		});
		await expect
			.poll(
				() =>
					canvas.evaluate((el) => {
						const source = el as HTMLCanvasElement;
						const probe = document.createElement("canvas");
						probe.width = source.width;
						probe.height = source.height;
						const context = probe.getContext("2d");
						if (context === null) return null;
						context.drawImage(source, 0, 0);
						const at = (fx: number): number[] =>
							Array.from(
								context.getImageData(
									Math.floor(source.width * fx),
									Math.floor(source.height / 2),
									1,
									1,
								).data,
							);
						const [lr = 0, lg = 0, lb = 0] = at(0.25);
						const [rr = 0, rg = 0, rb = 0] = at(0.75);
						return {
							// The overlay is transparent on the left: the green layer shows.
							leftIsGreen: lg > 90 && lr < 60 && lb < 60,
							// And opaque red on the right.
							rightIsRed: rr > 200 && rg < 60 && rb < 60,
						};
					}),
				{ timeout: 20_000 },
			)
			.toEqual({ leftIsGreen: true, rightIsRed: true });
		expect(failures).toEqual([]);
	});
});
