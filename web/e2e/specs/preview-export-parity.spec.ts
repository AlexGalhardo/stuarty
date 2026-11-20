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
import { nestedProject, pictureGroupProject } from "../support/nested";
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
// No case has a gate of its own: the effects that work on a layer's own pixels
// (grain, the channel split) match because the preview delivers a layer at the
// size the Go host delivers it (SDD-013 section 5.1 item 4).

test.describe("preview-export-parity", () => {
	test.use({ viewport: { width: 1920, height: 1300 } });
	test.describe.configure({ timeout: 300_000 });
	test.beforeEach(({ browserName }) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON); // verify:env-skip the browser cannot decode the proxy
	});

	// Epic 52: a composition drawn as a group (SDD-013 section 3.12), one level
	// and two, is drawn by the worker on a surface of its own, and the frame matches
	// the export's, which draws the same group.
	for (const levels of [1, 2] as const) {
		test(`nested ${levels === 1 ? "one level" : "two levels"}: the group is drawn as the export draws it (SSIM of at least ${MIN_SSIM}, at most ${MAX_DISPLACEMENT} px)`, async ({
			page,
			request,
		}) => {
			const project = await nestedProject(request, { levels });
			await openCase(page, project.id, "?executor=rust");
			await expect(canvasOf(page)).toHaveAttribute("data-executor", "rust");
			await stepThrough(page, project.frames, async (frame) => {
				await compareFrame(
					page,
					`nested-${levels}`,
					project.id,
					frame,
					"preview-export-parity",
					true,
				);
			});
		});
	}

	// A group of another timeline of the production that holds a picture wider
	// than the poster: the preview cuts the poster with the size of the picture,
	// which only the nested timeline's document names, and the picture's edges
	// let the displacement of the group be measured.
	test(`group of a wide picture of another timeline: drawn as the export draws it (SSIM of at least ${MIN_SSIM}, at most ${MAX_DISPLACEMENT} px)`, async ({
		page,
		request,
	}) => {
		const project = await pictureGroupProject(request);
		await openCase(page, project.id, "?executor=rust");
		await expect(canvasOf(page)).toHaveAttribute("data-executor", "rust");
		await stepThrough(page, project.frames, async (frame) => {
			await compareFrame(
				page,
				"group-picture",
				project.id,
				frame,
				"preview-export-parity",
				true,
			);
		});
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
				);
			});
		});
	}
});
