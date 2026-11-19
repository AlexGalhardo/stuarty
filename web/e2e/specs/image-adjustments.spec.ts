import type { Page } from "@playwright/test";

import {
	editImage,
	expect,
	type ImageLayer,
	imageState,
	layerByName,
	newImage,
	test,
} from "./image-support";

// Epic 29 task 9: the inspector controls for the adjustments, the filters, the mask and the blend mode of a
// layer. Every control sends one command of SDD-021 section 3.1; the spec reads what the server stored.

async function seed(request: Parameters<typeof newImage>[0]) {
	const { id, call } = await newImage(request, { name: "Adjustments spec" });
	await editImage(call, id, [
		{ op: "layer.add_fill", args: { color: "#203040", name: "Back" } },
		{
			op: "layer.add_shape",
			args: {
				shape: "rect",
				w: 400,
				h: 300,
				x: 300,
				y: 200,
				fill: "#FF8800",
				name: "Box",
			},
		},
		{
			op: "layer.add_shape",
			args: {
				shape: "ellipse",
				w: 200,
				h: 200,
				x: 800,
				y: 100,
				fill: "#00AAFF",
				name: "Dot",
			},
		},
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

test.describe("image adjustments, filters, masks and blend modes", () => {
	test.describe.configure({ timeout: 90_000 });

	test("the adjustment fields send layer.adjust and the reset removes them", async ({
		page,
		request,
	}) => {
		const { id, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Box").click();
		await expect(page.getByTestId("image-adjust-fields")).toBeVisible();

		await field(page, "image-adjust-brightness", "15");
		await expect
			.poll(async () => (await layer("Box")).adjust)
			.toEqual({ brightness: 15 });
		await field(page, "image-adjust-contrast", "-20");
		await field(page, "image-adjust-exposure", "50");
		await field(page, "image-adjust-levels-gamma", "120");
		await field(page, "image-adjust-curve", "64:48, 192:208");
		await expect
			.poll(async () => (await layer("Box")).adjust)
			.toEqual({
				brightness: 15,
				contrast: -20,
				exposure: 50,
				levels: { black: 0, white: 255, gamma: 120 },
				curve: [
					{ x: 64, y: 48 },
					{ x: 192, y: 208 },
				],
			});

		// A curve that is not a list of points is not sent and the field shows the stored one again.
		await field(page, "image-adjust-curve", "banana");
		await expect(page.getByTestId("image-adjust-curve")).toHaveValue(
			"64:48, 192:208",
		);

		await page.getByTestId("image-adjust-reset").click();
		await expect.poll(async () => (await layer("Box")).adjust).toBeUndefined();
	});

	test("filters are added, changed, moved and removed, in the order of the list", async ({
		page,
		request,
	}) => {
		const { id, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Box").click();
		const kinds = async (): Promise<string[]> =>
			((await layer("Box")).filters as { kind: string }[] | undefined)?.map(
				(f) => f.kind,
			) ?? [];

		await page.getByTestId("image-filter-add-kind").selectOption("blur");
		await page.getByTestId("image-filter-add").click();
		await expect.poll(kinds).toEqual(["blur"]);
		await page.getByTestId("image-filter-add-kind").selectOption("shadow");
		await page.getByTestId("image-filter-add").click();
		await expect.poll(kinds).toEqual(["blur", "shadow"]);

		await field(page, "image-filter-blur-radius", "20");
		await expect
			.poll(
				async () =>
					(
						(await layer("Box")).filters as {
							kind: string;
							radius?: number;
						}[]
					)[0]?.radius,
			)
			.toBe(20);
		await field(page, "image-filter-shadow-color", "#112233");
		await expect
			.poll(
				async () =>
					(
						(await layer("Box")).filters as {
							kind: string;
							color?: string;
						}[]
					)[1]?.color,
			)
			.toBe("#112233");

		await page.getByTestId("image-filter-shadow-up").click();
		await expect.poll(kinds).toEqual(["shadow", "blur"]);
		await expect(page.getByTestId("image-filter-shadow-up")).toBeDisabled();

		await page.getByTestId("image-filter-blur-remove").click();
		await expect.poll(kinds).toEqual(["shadow"]);
		// A kind that is on the layer is no longer offered.
		await expect(
			page.getByTestId("image-filter-add-kind").locator("option[value=shadow]"),
		).toHaveCount(0);
	});

	test("the mask is a shape in thousandths of the box, or the shape of another layer", async ({
		page,
		request,
	}) => {
		const { id, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Box").click();

		await page.getByTestId("image-mask-kind").selectOption("rounded_rect");
		await expect
			.poll(async () => (await layer("Box")).mask)
			.toMatchObject({ kind: "rounded_rect", w: 1000, h: 1000 });
		await field(page, "image-mask-w", "600");
		await field(page, "image-mask-radius", "24");
		await field(page, "image-mask-feather", "6");
		await page.getByTestId("image-mask-invert").click();
		await expect
			.poll(async () => (await layer("Box")).mask)
			.toMatchObject({
				kind: "rounded_rect",
				w: 600,
				radius: 24,
				feather: 6,
				invert: true,
			});

		await page.getByTestId("image-mask-kind").selectOption("layer");
		await expect
			.poll(async () => (await layer("Box")).mask)
			.toMatchObject({ kind: "layer" });
		const dot = await layer("Dot");
		await page.getByTestId("image-mask-source").selectOption(dot.id);
		await expect
			.poll(async () => (await layer("Box")).mask)
			.toMatchObject({ kind: "layer", source: dot.id });

		await page.getByTestId("image-mask-kind").selectOption("none");
		await expect.poll(async () => (await layer("Box")).mask).toBeUndefined();
	});

	test("the blend mode is one of the eleven, and normal removes it", async ({
		page,
		request,
	}) => {
		const { id, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Box").click();
		await expect(
			page.getByTestId("image-blend-mode").locator("option"),
		).toHaveCount(11);

		await page.getByTestId("image-blend-mode").selectOption("soft_light");
		await expect
			.poll(async () => (await layer("Box")).blend)
			.toBe("soft_light");
		await page.getByTestId("image-blend-mode").selectOption("normal");
		await expect.poll(async () => (await layer("Box")).blend).toBeUndefined();
	});

	test("what an agent does to the look shows in the fields, and a locked layer has them off", async ({
		page,
		request,
	}) => {
		const { id, call, layer } = await seed(request);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Box").click();
		const box = await layer("Box");

		await editImage(call, id, [
			{
				op: "layer.adjust",
				args: { layer_id: box.id, saturation: -40, hue: 25 },
			},
			{
				op: "layer.filter_add",
				args: { layer_id: box.id, kind: "grain", amount: 60 },
			},
			{ op: "layer.blend", args: { layer_id: box.id, mode: "multiply" } },
		]);
		await expect(page.getByTestId("image-adjust-saturation")).toHaveValue(
			"-40",
		);
		await expect(page.getByTestId("image-adjust-hue")).toHaveValue("25");
		await expect(page.getByTestId("image-filter-grain-amount")).toHaveValue(
			"60",
		);
		await expect(page.getByTestId("image-blend-mode")).toHaveValue("multiply");

		await editImage(call, id, [
			{ op: "layer.set", args: { layer_id: box.id, locked: true } },
		]);
		await expect(page.getByTestId("image-adjust-hue")).toBeDisabled();
		await expect(page.getByTestId("image-blend-mode")).toBeDisabled();
		await expect(page.getByTestId("image-filter-add")).toBeDisabled();
	});

	test("a group has no look fields", async ({ page, request }) => {
		const { id, call } = await seed(request);
		const state = await imageState(call, id);
		const ids = state.layers
			.filter((l) => l.name === "Box" || l.name === "Dot")
			.map((l) => l.id);
		await editImage(call, id, [
			{ op: "layer.group", args: { layer_ids: ids, name: "Pair" } },
		]);
		await page.goto(`/edit-image/${id}`);
		await row(page, "Pair").click();
		await expect(page.getByTestId("image-layer-fields")).toBeVisible();
		await expect(page.getByTestId("image-adjust-fields")).toHaveCount(0);
		await expect(page.getByTestId("image-blend-fields")).toHaveCount(0);
	});
});
