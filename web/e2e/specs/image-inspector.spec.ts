import type { Page } from "@playwright/test";

import {
	editImage,
	expect,
	type ImageLayer,
	imageState,
	layerByName,
	newImage,
	seedPhoto,
	test,
} from "./image-support";

// Epic 28 task 5: the inspector for the document and for image, text, shape and fill layers, and the buttons
// that add them. Every field sends one command; the spec reads what the server stored.

async function seed(request: Parameters<typeof newImage>[0], photo?: string) {
	const { id, call } = await newImage(request, { name: "Inspector spec" });
	await editImage(call, id, [
		{ op: "layer.add_fill", args: { color: "#203040", name: "Back" } },
		{
			op: "layer.add_shape",
			args: {
				shape: "rect",
				w: 200,
				h: 100,
				x: 300,
				y: 200,
				fill: "#FF8800",
				name: "Box",
			},
		},
		{
			op: "layer.add_text",
			args: { text: "HELLO", x: 100, y: 400, w: 600, name: "Title" },
		},
		...(photo === undefined
			? []
			: [
					{
						op: "layer.add_image",
						args: {
							asset: photo,
							name: "Photo",
							x: 700,
							y: 50,
							w: 300,
							h: 200,
						},
					},
				]),
	]);
	const layer = async (name: string): Promise<ImageLayer> => {
		const found = layerByName((await imageState(call, id)).layers, name);
		if (!found) throw new Error(`no layer ${name}`);
		return found;
	};
	return { id, call, layer };
}

const row = (page: Page, name: string) =>
	page.getByRole("treeitem", { name: new RegExp(`^${name},`) });

async function field(page: Page, testId: string, value: string): Promise<void> {
	const input = page.getByTestId(testId);
	await input.fill(value);
	await input.press("Enter");
}

test.describe("image inspector", () => {
	test.describe.configure({ timeout: 90_000 });

	test("with nothing selected it shows the canvas size and resizes it, keeping or scaling the layers", async ({
		page,
		request,
	}) => {
		const { id, call, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByTestId("image-doc-fields")).toBeVisible();
		await field(page, "image-canvas-width", "1000");
		await expect
			.poll(async () => (await imageState(call, id)).width)
			.toBe(1000);
		expect((await layer("Box")).x).toBe(300);

		await page.getByTestId("image-resize-mode").selectOption("scale");
		await field(page, "image-canvas-width", "500");
		await expect.poll(async () => (await imageState(call, id)).width).toBe(500);
		// scaled by 500/1000
		await expect.poll(async () => (await layer("Box")).x).toBe(150);
	});

	test("the common and box fields take pixels, px and percent of the canvas", async ({
		page,
		request,
	}) => {
		const { id, call, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Box").click();
		await expect(page.getByTestId("image-layer-fields")).toBeVisible();

		await field(page, "image-field-name", "Orange");
		await expect
			.poll(
				async () =>
					layerByName((await imageState(call, id)).layers, "Orange")?.id,
			)
			.toBeTruthy();
		await field(page, "image-field-opacity", "50%");
		await expect.poll(async () => (await layer("Orange")).opacity).toBe(50);
		await field(page, "image-field-x", "10%");
		await expect.poll(async () => (await layer("Orange")).x).toBe(128);
		await field(page, "image-field-y", "20px");
		await expect.poll(async () => (await layer("Orange")).y).toBe(20);
		await field(page, "image-field-w", "250");
		await expect.poll(async () => (await layer("Orange")).w).toBe(250);
		await field(page, "image-field-rotation", "30");
		await expect.poll(async () => (await layer("Orange")).rotation).toBe(30);

		await page.getByTestId("image-field-visible").click();
		await expect.poll(async () => (await layer("Orange")).visible).toBe(false);
		await page.getByTestId("image-field-locked").click();
		await expect.poll(async () => (await layer("Orange")).locked).toBe(true);
		// a locked layer's fields are off, except the lock itself
		await expect(page.getByTestId("image-field-x")).toBeDisabled();
		await expect(page.getByTestId("image-field-locked")).toBeEnabled();
	});

	test("text that is no length is refused in the field, and a value the core refuses falls back with its message", async ({
		page,
		request,
	}) => {
		const { id, call, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Box").click();
		const before = (await imageState(call, id)).revision;

		await field(page, "image-field-x", "abc");
		await expect(
			page.getByRole("alert").filter({ hasText: "whole number" }),
		).toBeVisible();
		expect((await imageState(call, id)).revision).toBe(before);

		await field(page, "image-field-w", "0");
		await expect(page.getByTestId("image-error")).toBeVisible();
		await expect(page.getByTestId("image-field-w")).toHaveValue("200");
		expect((await layer("Box")).w).toBe(200);
	});

	test("a shape has its kind, fill, outline and, rounded, its radius", async ({
		page,
		request,
	}) => {
		const { id, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Box").click();
		await expect(page.getByTestId("image-field-radius")).toHaveCount(0);
		await page.getByTestId("image-field-shape").selectOption("rounded_rect");
		await expect
			.poll(async () => (await layer("Box")).shape)
			.toBe("rounded_rect");
		await field(page, "image-field-radius", "12");
		await expect.poll(async () => (await layer("Box")).radius).toBe(12);
		await field(page, "image-field-fill", "#112233");
		await expect.poll(async () => (await layer("Box")).fill).toBe("#112233");
		await field(page, "image-field-stroke", "#FFFFFF");
		await field(page, "image-field-stroke-width", "4");
		await expect.poll(async () => (await layer("Box")).stroke_width).toBe(4);
		await expect
			.poll(async () => (await layer("Box")).stroke_color)
			.toBe("#FFFFFF");
		// an empty outline colour removes it
		await field(page, "image-field-stroke", "");
		await expect
			.poll(async () => (await layer("Box")).stroke_color)
			.toBeFalsy();
	});

	test("a fill layer has its colour", async ({ page, request }) => {
		const { id, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Back").click();
		await field(page, "image-field-fill", "#000000");
		await expect.poll(async () => (await layer("Back")).fill).toBe("#000000");
	});

	test("a text layer has its content, font, size, colour, alignment and case", async ({
		page,
		request,
	}) => {
		const { id, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Title").click();
		const text = page.getByTestId("image-field-text");
		await text.fill("BIGGER NEWS");
		await text.blur();
		await expect
			.poll(async () => (await layer("Title")).text)
			.toBe("BIGGER NEWS");
		await field(page, "image-field-size", "90");
		await expect.poll(async () => (await layer("Title")).style?.size).toBe(90);
		await field(page, "image-field-color", "#FFE600");
		await expect
			.poll(async () => (await layer("Title")).style?.color)
			.toBe("#FFE600");
		await page.getByTestId("image-field-text-align").selectOption("center");
		await expect
			.poll(async () => (await layer("Title")).style?.align)
			.toBe("center");
		await page.getByTestId("image-field-uppercase").click();
		await expect
			.poll(async () => (await layer("Title")).style?.uppercase)
			.toBe(true);
		await page.getByTestId("image-field-weight").selectOption("700");
		await expect
			.poll(async () => (await layer("Title")).style?.font_weight)
			.toBe(700);
	});

	test("an image layer has its crop, fit and a photo swap that keeps its place and slot", async ({
		page,
		request,
	}) => {
		const first = await seedPhoto(request, 0);
		const second = await seedPhoto(request, 1);
		const { id, call, layer } = await seed(request, first.hash);
		await editImage(call, id, [
			{
				op: "layer.set",
				args: { layer_id: (await layer("Photo")).id, slot: "photo" },
			},
		]);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Photo").click();
		await expect(page.getByTestId("image-image-fields")).toBeVisible();

		await page.getByTestId("image-fit-cover").click();
		await expect.poll(async () => (await layer("Photo")).w).toBeGreaterThan(0);
		await field(page, "image-crop-w", "320");
		await expect.poll(async () => (await layer("Photo")).crop?.w).toBe(320);
		await page.getByTestId("image-crop-reset").click();
		await expect.poll(async () => (await layer("Photo")).crop).toBeUndefined();

		const old = await layer("Photo");
		await page.getByTestId("image-field-asset").selectOption(second.hash);
		await expect
			.poll(async () => (await layer("Photo")).asset)
			.toBe(second.hash);
		const swapped = await layer("Photo");
		expect(swapped.id).not.toBe(old.id);
		expect(swapped.slot).toBe("photo");
		// same place in the stack: still above the text? no: the photo was the top layer and still is
		const names = (await imageState(call, id)).layers.map((l) => l.name);
		expect(names.at(-1)).toBe("Photo");
		// and the swap is one step of the history
		await page.getByTestId("image-undo").click();
		await expect
			.poll(async () => (await layer("Photo")).asset)
			.toBe(first.hash);
	});

	test("two layers align to the canvas and to a layer", async ({
		page,
		request,
	}) => {
		const { id, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Box").click();
		await row(page, "Title").click({ modifiers: ["Control"] });
		await page.getByTestId("image-align-left").click();
		await expect.poll(async () => (await layer("Box")).x).toBe(0);
		await expect.poll(async () => (await layer("Title")).x).toBe(0);

		await page.getByTestId("image-align-to").selectOption({ label: "Back" });
		await page.getByTestId("image-align-center").click();
		const back = await layer("Back");
		await expect
			.poll(async () => {
				const box = await layer("Box");
				return box.x + box.w / 2;
			})
			.toBe(back.x + back.w / 2);
	});

	test("the toolbar adds a text, shapes, a fill and an image of the library, and selects what it adds", async ({
		page,
		request,
	}) => {
		const photo = await seedPhoto(request, 0);
		const { id, call } = await newImage(request, { name: "Add spec" });
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByTestId("image-add-text")).toBeEnabled();

		await page.getByTestId("image-add-text").click();
		await expect(page.getByTestId("image-text-fields")).toBeVisible();
		await page.getByTestId("image-add-rect").click();
		await expect(page.getByTestId("image-shape-fields")).toBeVisible();
		await page.getByTestId("image-add-ellipse").click();
		await page.getByTestId("image-add-fill").click();
		await page.getByTestId("image-add-image").click();
		await page
			.getByTestId("image-picker-item")
			.filter({ hasText: photo.name.split("/").pop() ?? "" })
			.first()
			.click();
		await expect(page.getByTestId("image-image-fields")).toBeVisible();

		await expect
			.poll(async () => (await imageState(call, id)).layers.map((l) => l.kind))
			.toEqual(["text", "shape", "shape", "fill", "image"]);
		await expect(page.getByTestId("image-selection")).toHaveCount(1);
	});
});
