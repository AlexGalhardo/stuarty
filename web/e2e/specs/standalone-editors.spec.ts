import {
	caller,
	expect,
	listProjects,
	test,
	testClip,
	validName,
} from "./helpers";
import { imageState, seedPhoto, testPhoto } from "./image-support";

// Epic 105: standalone video (/edit-video) and image (/edit-image) editors without a pre-created project.
test.describe("standalone editors", () => {
	test.describe.configure({ timeout: 240_000 });

	test("drops a video on /edit-video, edits and exports without a project, then promotes to a project", async ({
		page,
		request,
	}) => {
		await page.goto("/edit-video");
		await expect(page.getByTestId("edit-video-dropzone")).toBeVisible();
		await expect(page.getByTestId("edit-video-presets")).toBeVisible();

		// Upload a real video file directly on the standalone launcher (no project created).
		await page.getByTestId("edit-video-file-input").setInputFiles(testClip(0));
		await page.waitForURL(/\/editor\/prj_/, { timeout: 90_000 });
		const id = page.url().split("/").pop() ?? "";
		expect(id).toMatch(/^prj_/);

		// Clip is already placed on V1 and the standalone "Save as Project" button is shown.
		await expect(page.locator("[data-item-id]")).toHaveCount(1, {
			timeout: 60_000,
		});
		await expect(page.getByTestId("editor-promote-project")).toBeVisible();

		// Standalone edits stay isolated from /projects until promoted.
		const before = await listProjects(request);
		expect(before.some((p) => p.id === id)).toBe(false);

		// Export the standalone video directly.
		await page.getByRole("button", { name: "Export", exact: true }).click();
		const exportDialog = page.getByRole("dialog");
		await exportDialog
			.getByRole("button", { name: /^(Export 1 file|Start export)$/ })
			.click();
		await expect(
			exportDialog.getByRole("link", { name: /^Download/ }).first(),
		).toBeVisible({ timeout: 180_000 });
		await page.keyboard.press("Escape");

		// Promote the standalone edit into a full project with SMART goal.
		const promotedName = validName("Promoted cut");
		await page.getByTestId("editor-promote-project").click();
		const dialog = page.getByTestId("promote-project-dialog");
		await expect(dialog).toBeVisible();
		await dialog.locator("#promote-name").fill(promotedName);
		await dialog
			.locator("#promote-description")
			.fill(
				"A standalone edit promoted into a full production project with all timeline edits kept intact for publishing.",
			);
		await dialog
			.locator("#promote-specific")
			.fill("Publish one short video cut from the standalone edit");
		await dialog
			.locator("#promote-measurable")
			.fill("Export finishes with zero errors and appears in Projects");
		await dialog
			.locator("#promote-achievable")
			.fill("Footage is already cut and verified in the timeline");
		await dialog
			.locator("#promote-relevant")
			.fill("Turns quick standalone edits into full productions when needed");
		await dialog
			.locator("#promote-time-bound")
			.fill("Export today and publish this week");
		await dialog.getByTestId("promote-project-submit").click();

		await expect(dialog).toBeHidden({ timeout: 30_000 });
		await expect(page.getByTestId("editor-promote-project")).toBeHidden();

		const after = await listProjects(request);
		expect(after.some((p) => p.id === id && p.name === promotedName)).toBe(
			true,
		);
	});

	test("drops an image on /edit-image top dropzone and exports PNG with zero modal or project", async ({
		page,
		request,
	}) => {
		await page.goto("/edit-image");
		await expect(page.getByTestId("edit-image-dropzone")).toBeVisible();

		await page.getByTestId("edit-image-file-input").setInputFiles(testPhoto(0));
		await page.waitForURL(/\/edit-image\/img_/, { timeout: 60_000 });
		const id = page.url().split("/").pop() ?? "";
		expect(id).toMatch(/^img_/);

		await expect(page.getByTestId("image-frame")).toBeVisible({
			timeout: 30_000,
		});
		await expect(page.getByTestId("image-attach-project")).toBeVisible();

		const call = await caller(request, "agent");
		const state = await imageState(call, id);
		expect(state.layers.map((l) => l.kind)).toEqual(["image"]);

		// Export PNG directly.
		await page.getByTestId("image-export-open").click();
		const dialog = page.getByTestId("image-export-dialog");
		await expect(dialog).toBeVisible();
		await dialog.getByTestId("image-export-run").click();
		await expect(page.getByTestId("image-export-result")).toBeVisible({
			timeout: 60_000,
		});
	});

	test("?asset=<hash> opens /edit-video and /edit-image pre-loaded with workspace assets", async ({
		page,
		request,
	}) => {
		// Upload one video via standalone launcher and seed one photo into the workspace library.
		await page.goto("/edit-video");
		await page.getByTestId("edit-video-file-input").setInputFiles(testClip(1));
		await page.waitForURL(/\/editor\/prj_/, { timeout: 90_000 });
		const photo = await seedPhoto(request, 1);

		const call = await caller(request);
		const videos = (await call("GET", "/assets?kind=video")) as {
			assets: { hash: string }[];
		};
		const videoHash = videos.assets[0]?.hash ?? "";
		const imageHash = photo.hash;
		expect(videoHash).toMatch(/^sha256:/);
		expect(imageHash).toMatch(/^sha256:/);

		await page.goto(`/edit-video?asset=${encodeURIComponent(videoHash)}`);
		await page.waitForURL(/\/editor\/prj_/, { timeout: 90_000 });
		await expect(page.locator("[data-item-id]")).toHaveCount(1, {
			timeout: 60_000,
		});

		await page.goto(`/edit-image?asset=${encodeURIComponent(imageHash)}`);
		await page.waitForURL(/\/edit-image\/img_/, { timeout: 60_000 });
		await expect(page.getByTestId("image-frame")).toBeVisible({
			timeout: 30_000,
		});
	});
});
