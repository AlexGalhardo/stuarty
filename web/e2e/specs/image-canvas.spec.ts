import { editImage, expect, imageState, newImage, test } from "./image-support";

// Epic 28 task 2: the page, the loading of the document and the canvas with zoom and pan.
test.describe("image canvas", () => {
	test.describe.configure({ timeout: 90_000 });
	test("opens a document, draws the picture of its revision and shows its name", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Canvas spec" });
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#204060" } },
		]);
		const state = await imageState(call, id);

		await page.goto(`/edit-image/${id}`);
		await expect(
			page.getByRole("heading", { level: 1, name: "Canvas spec" }),
		).toBeVisible();
		const frame = page.getByTestId("image-frame");
		await expect(frame).toBeVisible();
		await expect(frame).toHaveAttribute(
			"data-revision",
			String(state.revision),
		);
		await expect
			.poll(() => frame.evaluate((el: HTMLImageElement) => el.naturalWidth))
			.toBeGreaterThan(0);
	});

	test("the revision of the picture is the one drawn, not the one still loading", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, {
			name: "Frame revision spec",
		});
		await page.goto(`/edit-image/${id}`);
		const frame = page.getByTestId("image-frame");
		await expect(frame).toHaveAttribute("data-revision", "1");

		// the picture of the next revision is held back until the spec lets it go
		let release: () => void = () => undefined;
		const held = new Promise<void>((resolve) => {
			release = resolve;
		});
		await page.route(/\/images\/[^/]+\/frame\?/, async (route) => {
			await held;
			await route.continue();
		});
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#204060", name: "Held" } },
		]);
		await expect(page.getByRole("treeitem", { name: /^Held,/ })).toBeVisible();
		await expect(frame).toHaveAttribute("data-revision", "1");

		release();
		await expect(frame).toHaveAttribute("data-revision", "2");
		await page.unroute(/\/images\/[^/]+\/frame\?/);
	});

	test("zooms with the buttons and the keys, fits, and goes to 100%", async ({
		page,
		request,
	}) => {
		const { id } = await newImage(request);
		await page.goto(`/edit-image/${id}`);
		const stage = page.getByTestId("image-stage");
		await expect(page.getByTestId("image-frame")).toBeVisible({
			timeout: 30_000,
		});
		const zoom = async (): Promise<number> =>
			Number(await stage.getAttribute("data-zoom"));

		const fitted = await zoom();
		expect(fitted).toBeGreaterThan(0);
		await page.getByTestId("image-zoom-in").click();
		const bigger = await zoom();
		expect(bigger).toBeCloseTo(fitted * 1.25, 3);
		await page.getByTestId("image-zoom-out").click();
		await page.getByTestId("image-zoom-out").click();
		expect(await zoom()).toBeLessThan(fitted);

		await page.getByTestId("image-zoom-100").click();
		expect(await zoom()).toBeCloseTo(1, 4);
		await page.getByTestId("image-zoom-fit").click();
		expect(await zoom()).toBeCloseTo(fitted, 4);

		// The same by the keyboard, with the canvas focused.
		await stage.focus();
		await page.keyboard.press("+");
		expect(await zoom()).toBeCloseTo(fitted * 1.25, 3);
		await page.keyboard.press("1");
		expect(await zoom()).toBeCloseTo(1, 4);
		await page.keyboard.press("0");
		expect(await zoom()).toBeCloseTo(fitted, 4);
	});

	test("pans with Space and a drag, and zooms about the pointer with Ctrl and the wheel", async ({
		page,
		request,
	}) => {
		const { id } = await newImage(request);
		await page.goto(`/edit-image/${id}`);
		const stage = page.getByTestId("image-stage");
		const picture = page.getByTestId("image-picture");
		await expect(page.getByTestId("image-frame")).toBeVisible({
			timeout: 30_000,
		});

		const before = await picture.boundingBox();
		if (!before) throw new Error("no picture");
		await stage.focus();
		const box = await stage.boundingBox();
		if (!box) throw new Error("no stage");
		await page.keyboard.down(" ");
		await page.mouse.move(box.x + 100, box.y + 100);
		await page.mouse.down();
		await page.mouse.move(box.x + 160, box.y + 130, { steps: 4 });
		await page.mouse.up();
		await page.keyboard.up(" ");
		const after = await picture.boundingBox();
		if (!after) throw new Error("no picture");
		expect(after.x - before.x).toBeCloseTo(60, 0);
		expect(after.y - before.y).toBeCloseTo(30, 0);

		// zoom about the pointer: the point of the picture under it stays under it
		const px = after.x + after.width / 2;
		const py = after.y + after.height / 2;
		await page.mouse.move(px, py);
		await page.keyboard.down("Control");
		await page.mouse.wheel(0, -100);
		await page.keyboard.up("Control");
		const zoomed = await picture.boundingBox();
		if (!zoomed) throw new Error("no picture");
		expect(zoomed.width).toBeGreaterThan(after.width);
		expect(zoomed.x + zoomed.width / 2).toBeCloseTo(px, 0);
		expect(zoomed.y + zoomed.height / 2).toBeCloseTo(py, 0);
	});

	test("a document that does not exist says so and offers the way back", async ({
		page,
	}) => {
		await page.goto("/edit-image/img_0000000000");
		await expect(
			page
				.getByTestId("image-error")
				.or(
					page.getByRole("heading", { name: "The image could not be opened." }),
				),
		).toBeVisible();
		await expect(
			page.getByRole("link", { name: "Back to Edit Image" }),
		).toBeVisible();
	});

	test("the list opens a new image of a preset", async ({ page }) => {
		await page.goto("/edit-image");
		await page.getByLabel("Name").first().fill("From the list");
		await page.getByRole("button", { name: "Create image" }).click();
		await expect(page).toHaveURL(/\/edit-image\/img_/);
		await expect(
			page.getByRole("heading", { level: 1, name: "From the list" }),
		).toBeVisible();
		await expect(page.getByTestId("image-frame")).toBeVisible({
			timeout: 30_000,
		});
	});
});
