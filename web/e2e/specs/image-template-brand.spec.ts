import type { Page } from "@playwright/test";

import { caller } from "./helpers";
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

// Epic 28 task 6: the brand kit in the editor (swatches, "place logo") and "new from template" with the values
// of its slots. The kit belongs to the workspace, as in brand-kit.spec.ts, so these tests run one after another.
test.describe("image brand kit and templates", () => {
	test.describe.configure({ timeout: 90_000, mode: "serial" });

	const row = (page: Page, name: string) =>
		page.getByRole("treeitem", { name: new RegExp(`^${name},`) });

	test("the brand colours are swatches that set a colour, and the logo is placed in one click", async ({
		page,
		request,
	}) => {
		const logo = await seedPhoto(request, 2);
		const call = await caller(request);
		await call("PUT", "/brand-kit", {
			logo: logo.hash,
			colors: { brand: "#FF0066", ink: "#101010" },
		});
		const { id, call: agent } = await newImage(request, { name: "Brand spec" });
		await editImage(agent, id, [
			{
				op: "layer.add_shape",
				args: { shape: "rect", w: 200, h: 100, x: 100, y: 100, name: "Box" },
			},
			{ op: "layer.add_text", args: { text: "HELLO", name: "Title" } },
		]);
		const layer = async (name: string): Promise<ImageLayer> => {
			const found = layerByName((await imageState(agent, id)).layers, name);
			if (!found) throw new Error(`no layer ${name}`);
			return found;
		};

		await page.goto(`/edit-image/${id}`);
		await row(page, "Box").click();
		const swatch = page.getByRole("button", { name: "Use brand (#FF0066)" });
		await expect(swatch.first()).toBeVisible();
		await swatch.first().click();
		await expect.poll(async () => (await layer("Box")).fill).toBe("#FF0066");

		await row(page, "Title").click();
		await page
			.getByRole("button", { name: "Use ink (#101010)" })
			.first()
			.click();
		await expect
			.poll(async () => (await layer("Title")).style?.color)
			.toBe("#101010");

		await page.getByTestId("image-place-logo").click();
		await expect.poll(async () => (await layer("Logo")).asset).toBe(logo.hash);
		// the new layer is the selection
		await expect(page.getByTestId("image-selection")).toHaveAttribute(
			"data-layer",
			(await layer("Logo")).id,
		);
	});

	test("without a logo the place-logo button is off", async ({
		page,
		request,
	}) => {
		const call = await caller(request);
		await call("PUT", "/brand-kit", { colors: { brand: "#FF0066" } });
		const { id } = await newImage(request, { name: "No logo spec" });
		await page.goto(`/edit-image/${id}`);
		await expect(page.getByTestId("image-add-text")).toBeEnabled();
		await expect(page.getByTestId("image-place-logo")).toBeDisabled();
	});

	test("new from template asks for the slots, names what is missing and makes the image", async ({
		page,
		request,
	}) => {
		const first = await seedPhoto(request, 0);
		const second = await seedPhoto(request, 1);
		const made = await newImage(request, { name: "Template spec" });
		// make it a template: a new document with the template flag
		const created = (await made.call("POST", "/images", {
			name: "Thumbnail template",
			preset: "video_thumbnail",
			template: true,
		})) as { image_id: string };
		const tpl = created.image_id;
		await editImage(made.call, tpl, [
			{
				op: "layer.add_image",
				args: { asset: first.hash, slot: "photo", name: "Photo", fit: "cover" },
			},
			{
				op: "layer.add_text",
				args: { text: "", slot: "title", name: "Title", y: 500 },
			},
		]);

		await page.goto("/edit-image");
		const form = page.getByTestId("image-template-form");
		await expect(form).toBeVisible();
		await page
			.getByLabel("Template", { exact: true })
			.selectOption({ label: "Thumbnail template" });
		await expect(page.getByLabel(/Title .*required/)).toBeVisible();

		// no value for the required slot: the core says which one is missing
		await page.getByRole("button", { name: "Create from template" }).click();
		await expect(
			page.getByRole("alert").filter({ hasText: "title" }).first(),
		).toBeVisible();
		await expect(
			page.getByText("Fill the slots marked required"),
		).toBeVisible();

		await page.getByLabel(/Title .*required/).fill("EPISODE 12");
		await page.getByLabel(/Photo/).selectOption(second.hash);
		await page.getByLabel("Name").last().fill("Episode 12");
		await page.getByRole("button", { name: "Create from template" }).click();

		await expect(page).toHaveURL(/\/edit-image\/img_/);
		await expect(
			page.getByRole("heading", { level: 1, name: "Episode 12" }),
		).toBeVisible();
		const id = page.url().split("/").pop() ?? "";
		const state = await imageState(made.call, id);
		expect(layerByName(state.layers, "Title")?.text).toBe("EPISODE 12");
		expect(layerByName(state.layers, "Photo")?.asset).toBe(second.hash);
		await expect(page.getByTestId("image-frame")).toBeVisible();
		// the template itself is untouched and is not listed among the images
		await page.goto("/edit-image");
		await expect(
			page.getByTestId("image-list").getByText("Episode 12"),
		).toBeVisible();
		await expect(
			page.getByTestId("image-list").getByText("Thumbnail template"),
		).toHaveCount(0);
	});
});
