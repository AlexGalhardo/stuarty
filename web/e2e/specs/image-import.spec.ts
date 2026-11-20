import { caller } from "./helpers";
import { expect, imageState, newImage, test, testPhoto } from "./image-support";

// Epic 48 (SDD-021 section 10): a file of the computer becomes an image document, or one more layer of an open
// one, from the screen. The pictures are real files made by FFmpeg and read by the real ingest.
test.describe("image import", () => {
	test.describe.configure({ timeout: 120_000 });

	const imageList = async (
		request: Parameters<typeof caller>[0],
	): Promise<{ image_id: string; name: string }[]> => {
		const call = await caller(request, "agent");
		const list = (await call("GET", "/images")) as unknown as {
			images: { image_id: string; name: string }[];
		};
		return list.images;
	};

	test("a file chosen on the list becomes a new image the size of the file", async ({
		page,
		request,
	}) => {
		await page.goto("/edit-image");
		await page.getByTestId("image-import-open").click();
		const dialog = page.getByTestId("image-import-dialog");
		await dialog.getByTestId("image-import-file").setInputFiles(testPhoto(0));
		await dialog.getByTestId("image-import-submit").click();

		await page.waitForURL(/\/edit-image\/img_/);
		const id = page.url().split("/").pop() ?? "";
		const call = await caller(request, "agent");
		const state = await imageState(call, id);
		expect(state.width).toBe(640);
		expect(state.height).toBe(360);
		expect(state.layers.map((l) => l.kind)).toEqual(["image"]);
		await expect(
			page.getByRole("treeitem", { name: /^photo-0\.png,/ }),
		).toBeVisible();
		await expect(page.getByTestId("image-frame")).toBeVisible();
	});

	test("a size chosen for the new image fits the picture in it", async ({
		page,
		request,
	}) => {
		await page.goto("/edit-image");
		await page.getByTestId("image-import-open").click();
		const dialog = page.getByTestId("image-import-dialog");
		await dialog.getByTestId("image-import-file").setInputFiles(testPhoto(1));
		await dialog
			.getByLabel("Size of the new image")
			.selectOption("square_post");
		await dialog.getByTestId("image-import-submit").click();

		await page.waitForURL(/\/edit-image\/img_/);
		const id = page.url().split("/").pop() ?? "";
		const state = await imageState(await caller(request, "agent"), id);
		expect([state.width, state.height]).toEqual([1080, 1080]);
		expect(state.layers).toHaveLength(1);
	});

	test("a file put in an open image is one more layer, selected", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Put a file" });
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByTestId("image-frame")).toBeVisible();
		await page.getByTestId("image-import-open").click();
		const dialog = page.getByTestId("image-import-dialog");
		await dialog.getByTestId("image-import-file").setInputFiles(testPhoto(2));
		await dialog.getByTestId("image-import-submit").click();

		const layer = page.getByRole("treeitem", { name: /^photo-2\.png,/ });
		await expect(layer).toBeVisible();
		await expect(dialog).toBeHidden();
		await expect(layer).toHaveAttribute("aria-selected", "true");
		const state = await imageState(call, id);
		expect(state.layers.map((l) => l.kind)).toEqual(["image"]);
		expect(state.revision).toBe(2);
	});

	test("a path on the server is read too", async ({ page, request }) => {
		await page.goto("/edit-image");
		await page.getByTestId("image-import-open").click();
		const dialog = page.getByTestId("image-import-dialog");
		await dialog.getByTestId("image-import-path").fill(testPhoto(3));
		await dialog.getByTestId("image-import-submit").click();
		await page.waitForURL(/\/edit-image\/img_/);
		const id = page.url().split("/").pop() ?? "";
		const state = await imageState(await caller(request, "agent"), id);
		expect(state.layers).toHaveLength(1);
	});

	test("a file that is not an image is refused with the reason, and nothing is made", async ({
		page,
		request,
	}) => {
		await page.goto("/edit-image");
		await page.getByTestId("image-import-open").click();
		const dialog = page.getByTestId("image-import-dialog");
		await dialog.getByTestId("image-import-file").setInputFiles({
			name: "notes.png",
			mimeType: "image/png",
			buffer: Buffer.from(
				"<html><script>alert(1)</script></html> not a picture",
			),
		});
		await dialog.getByTestId("image-import-submit").click();

		await expect(dialog.getByRole("alert")).toContainText("not");
		await expect(page).toHaveURL(/\/edit-image$/);
		// Other specs make documents at the same time, so the check is on the name of this file only.
		expect(
			(await imageList(request)).filter((i) => i.name.startsWith("notes")),
		).toEqual([]);
	});

	test("nothing chosen asks for a file or a path before anything is sent", async ({
		page,
	}) => {
		await page.goto("/edit-image");
		await page.getByTestId("image-import-open").click();
		const dialog = page.getByTestId("image-import-dialog");
		await dialog.getByTestId("image-import-submit").click();
		await expect(dialog.getByRole("alert")).toContainText(
			"Choose a file or give a path",
		);
	});
});
