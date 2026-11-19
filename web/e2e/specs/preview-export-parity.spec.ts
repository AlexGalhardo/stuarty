import { NO_VIDEO_DECODE_REASON, noVideoDecode } from "../support/browsers";
import {
	compareFrame,
	goldenCases,
	loadCase,
	MAX_DISPLACEMENT,
	MIN_SSIM,
	needsImageFixtures,
	openCase,
	stepThrough,
} from "../support/golden";
import { canvasOf } from "../support/webcodecs";
import { expect, test } from "./helpers";

// Epic 26 task 6 (ADR-0029, SDD-013 section 5.4): the preview drawn by the
// renderer of the export (`?executor=rust`) against the export's own frame of the
// same plan, for every golden project of engine/export/testdata/golden. Each case
// is loaded with all its edits (the blend, the key, the flips, the mask, the
// effects, the shadow and the outline too), the preview is stepped to every listed
// frame, and the canvas is compared with ViewFrame at the same width: luma SSIM
// of at least 0.97 and no layer displaced by more than 1 px
// (ADR-0029, the tolerance of the CPU path against the export). A frame that
// misses is attached as the canvas, the export's frame and their difference.
// ViewFrame is a JPEG. Seeded grain is a blocky noise of one or two pixels and the
// split of the red and blue channels is a one-pixel colour fringe: JPEG removes
// both, so the luma SSIM of these two cases against it is lower than the
// 0.97 of the rest although the picture is the same (the difference
// picture shows the 8 x 8 blocks of the codec and no displacement: shift 0.04
// px). They keep a floor of their own, measured on 2026-11-19; the renderer's CPU
// path equals the export's compositor byte for byte (engine/export
// rust_renderer_test.go), so the gap is the reference's.
const JPEG_FLOOR: Record<string, number> = {
	"effect-grain": 0.9,
	"effect-rgb-split": 0.96,
};

test.describe("preview-export-parity", () => {
	test.use({ viewport: { width: 1920, height: 1300 } });
	test.describe.configure({ timeout: 300_000 });
	test.beforeEach(({ browserName }) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON); // verify:env-skip the browser cannot decode the proxy
	});

	for (const name of goldenCases()) {
		test(`golden ${name}: the renderer's frames match the export's (SSIM of at least ${MIN_SSIM}, at most ${MAX_DISPLACEMENT} px)`, async ({
			page,
			request,
		}) => {
			test.skip(needsImageFixtures(name), "fixtures only the Go suite makes"); // verify:env-skip an alpha PNG or a GIF with delays
			const loaded = await loadCase(request, name, true);
			await openCase(page, loaded.id, "?executor=rust");
			await expect(canvasOf(page)).toHaveAttribute("data-executor", "rust");
			await stepThrough(page, loaded.frames, async (frame) => {
				await compareFrame(
					page,
					name,
					loaded.id,
					frame,
					"preview-export-parity",
					JPEG_FLOOR[name] ?? MIN_SSIM,
				);
			});
		});
	}
});
