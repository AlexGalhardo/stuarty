import { expect, test } from "@playwright/test";

// P1-UI-003 asserts that need the render pipeline behind the plan and frame
// endpoints (PlanWindow and ViewFrame, P1-RND-005 and P1-SRV-006) and the
// three reference scenes. The unit tests of resources/js/editor/preview cover
// the plan windows, the sync rules, the audio clock and truth on pause.
const PENDING_RENDER =
	"waiting for the plan and frame endpoints (P1-RND-005, P1-SRV-006 ViewFrame) and the reference scenes";

test.describe("preview-vs-export", () => {
	test.skip(true, PENDING_RENDER);

	test("the canvas matches ViewFrame: SSIM of at least 0.97 on luma, no layer displaced over 1 pixel", async ({
		page,
	}) => {
		await page.goto("/editor/prj_0000000000");
		await expect(page.locator("canvas")).toBeVisible();
	});
});

test.describe("play-pause-truth", () => {
	test.skip(true, PENDING_RENDER);

	test("after pausing the exact frame appears within 500 ms at the 95th percentile (NFR-004)", async ({
		page,
	}) => {
		await page.goto("/editor/prj_0000000000");
		await page.getByRole("button", { name: "Play" }).click();
		await page.getByRole("button", { name: "Pause" }).click();
		await expect(page.getByAltText("Exact frame at the playhead")).toBeVisible({
			timeout: 500,
		});
	});
});

test.describe("csp-clean", () => {
	test.skip(true, PENDING_RENDER);

	test("no securitypolicyviolation event and no blob: URL during playback", async ({
		page,
	}) => {
		const violations: string[] = [];
		await page.exposeFunction("vvViolation", (v: string) => violations.push(v));
		await page.addInitScript(() => {
			document.addEventListener("securitypolicyviolation", (e) =>
				(window as unknown as { vvViolation(v: string): void }).vvViolation(
					e.violatedDirective,
				),
			);
		});
		await page.goto("/editor/prj_0000000000");
		await page.getByRole("button", { name: "Play" }).click();
		expect(violations).toEqual([]);
	});
});

test.describe("video-layers-budget", () => {
	test.skip(true, PENDING_RENDER);

	test("4, 6 and 8 simultaneous video layers drop under 2% of frames over 10 s", async () => {
		test.fixme(true, "measures MAX_VIDEO_LAYERS on the reference machine");
	});
});
