import AxeBuilder from "@axe-core/playwright";
import type { Page } from "@playwright/test";

import {
	editImage,
	expect,
	imageState,
	layerByName,
	newImage,
	seedPhoto,
	test,
} from "./image-support";

// Epic 28 task 8 and acceptance criteria 3 and 5: axe over the image editor in both languages and both themes,
// and a session with the keyboard only.
const TAGS = ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"];

async function seedAll(request: Parameters<typeof newImage>[0]) {
	const photo = await seedPhoto(request, 0);
	const { id, call } = await newImage(request, { name: "A11y spec" });
	await editImage(call, id, [
		{ op: "layer.add_fill", args: { color: "#203040", name: "Back" } },
		{
			op: "layer.add_image",
			args: { asset: photo.hash, name: "Photo", fit: "cover", slot: "photo" },
		},
		{
			op: "layer.add_shape",
			args: {
				shape: "rounded_rect",
				w: 300,
				h: 120,
				x: 100,
				y: 100,
				radius: 12,
				name: "Box",
			},
		},
		{
			op: "layer.add_text",
			args: {
				text: "HEADLINE",
				name: "Title",
				style: { size: 100, color: "#FFE600" },
			},
		},
	]);
	return { id, call };
}

const row = (page: Page, name: string) =>
	page.getByRole("treeitem", { name: new RegExp(`^${name},`) });

async function axeOf(page: Page, label: string): Promise<string[]> {
	const { violations } = await new AxeBuilder({ page })
		.withTags(TAGS)
		.analyze();
	return violations.map((v) => `${label}: ${v.id} (${v.nodes.length})`);
}

test.describe("a11y image editor", () => {
	test.describe.configure({ timeout: 180_000 });

	for (const lang of ["en", "pt-BR"] as const) {
		for (const theme of ["light", "dark"] as const) {
			test(`axe finds nothing on the image pages in ${theme} in ${lang}`, async ({
				page,
				request,
			}) => {
				const { id } = await seedAll(request);
				await page.addInitScript(
					([l, th]) => {
						localStorage.setItem("vv.lang", l);
						localStorage.setItem("theme", th);
					},
					[lang, theme],
				);
				const found: string[] = [];

				await page.goto("/edit-image");
				await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
				found.push(...(await axeOf(page, "list")));

				await page.goto(`/edit-image/${id}`);
				await expect(page.getByTestId("image-frame")).toBeVisible({
					timeout: 60_000,
				});
				found.push(...(await axeOf(page, "editor, nothing selected")));

				for (const name of ["Back", "Photo", "Box", "Title"]) {
					await page
						.getByRole("treeitem", { name: new RegExp(`^${name}`) })
						.click();
					await expect(page.getByTestId("image-layer-fields")).toBeVisible();
					found.push(...(await axeOf(page, `editor, ${name} selected`)));
				}

				await page.getByTestId("image-export-open").click();
				await expect(page.getByTestId("image-export-dialog")).toBeVisible();
				found.push(...(await axeOf(page, "export dialog")));
				await page.keyboard.press("Escape");
				await expect(page.getByTestId("image-export-dialog")).toHaveCount(0);

				await page.getByTestId("image-add-image").click();
				await expect(page.getByTestId("image-picker")).toBeVisible();
				found.push(...(await axeOf(page, "image picker")));
				await page.keyboard.press("Escape");

				expect(found).toEqual([]);
			});
		}
	}

	// The next focus stop that satisfies `test`, by Tab alone.
	async function tabTo(
		page: Page,
		test: (el: Element) => boolean,
		key = "Tab",
	): Promise<void> {
		for (let i = 0; i < 120; i++) {
			await page.keyboard.press(key);
			const hit = await page.evaluate(
				`(${test.toString()})(document.activeElement)`,
			);
			if (hit) return;
		}
		throw new Error("the keyboard never reached the control");
	}

	test("a person can build and change a thumbnail with the keyboard only", async ({
		page,
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "Keyboard spec" });
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#203040", name: "Back" } },
		]);
		const layer = async (name: string) =>
			layerByName((await imageState(call, id)).layers, name);
		await page.goto(`/edit-image/${id}`);
		await expect(row(page, "Back")).toBeVisible();

		// The keys start at the first control of the editor: the sidebar before it is the dashboard's, and the
		// focus of its language control raises a CSP violation that is not this editor's (VV-249).
		await page.getByRole("link", { name: "Back to Edit Image" }).focus();
		// the toolbar is one tab stop and the arrows walk it: add a text with Enter
		await tabTo(page, (el) => el.hasAttribute("data-tool"));
		await tabTo(
			page,
			(el) => el.getAttribute("data-testid") === "image-add-text",
			"ArrowRight",
		);
		await page.keyboard.press("Enter");
		await expect.poll(async () => (await layer("Text"))?.kind).toBe("text");

		// the layer tree: rename with F2, move with Alt and the arrows
		await tabTo(page, (el) => el.getAttribute("role") === "treeitem");
		await expect(page.getByRole("treeitem", { selected: true })).toBeFocused();
		await page.keyboard.press("F2");
		await page.keyboard.type("Headline");
		await page.keyboard.press("Enter");
		await expect.poll(async () => (await layer("Headline"))?.kind).toBe("text");
		await page.keyboard.press("Alt+ArrowDown");
		await expect
			.poll(async () =>
				(await imageState(call, id)).layers.map((l) => l.name).join(","),
			)
			.toBe("Headline,Back");

		// the canvas: arrows nudge, Shift makes it ten, PageUp picks the next layer
		await tabTo(page, (el) => el.getAttribute("data-testid") === "image-stage");
		await page.keyboard.press("PageUp");
		await page.keyboard.press("PageDown");
		const start = await layer("Headline");
		await page.keyboard.press("ArrowRight");
		await page.keyboard.press("Shift+ArrowDown");
		await expect
			.poll(async () => (await layer("Headline"))?.x)
			.toBe((start?.x ?? 0) + 1);
		await expect
			.poll(async () => (await layer("Headline"))?.y)
			.toBe((start?.y ?? 0) + 10);

		// the inspector: the fields are in the tab order, Enter sends
		await tabTo(
			page,
			(el) => el.getAttribute("data-testid") === "image-field-name",
		);
		await page.keyboard.press("Control+a");
		await page.keyboard.type("Title");
		await page.keyboard.press("Enter");
		await expect.poll(async () => (await layer("Title"))?.kind).toBe("text");
		await tabTo(
			page,
			(el) => el.getAttribute("data-testid") === "image-field-opacity",
		);
		await page.keyboard.press("Control+a");
		await page.keyboard.type("40%");
		await page.keyboard.press("Enter");
		await expect.poll(async () => (await layer("Title"))?.opacity).toBe(40);

		// undo from the canvas, then the export dialog opens and closes by keys
		await page.getByTestId("image-stage").focus();
		await page.keyboard.press("Control+z");
		await expect.poll(async () => (await layer("Title"))?.opacity).toBe(100);
		await tabTo(page, (el) => el.hasAttribute("data-tool"), "Shift+Tab");
		await tabTo(
			page,
			(el) => el.getAttribute("data-testid") === "image-export-open",
			"ArrowRight",
		);
		await page.keyboard.press("Enter");
		await expect(page.getByTestId("image-export-dialog")).toBeVisible();
		await page.keyboard.press("Escape");
		await expect(page.getByTestId("image-export-dialog")).toHaveCount(0);
		await expect(page.getByTestId("image-export-open")).toBeFocused();
	});

	test("every operation of the catalogue has a control: the fields and buttons that send them", async ({
		page,
		request,
	}) => {
		const { id } = await seedAll(request);
		await page.goto(`/edit-image/${id}`);
		await expect(row(page, "Photo")).toBeVisible();
		const present = async (testId: string): Promise<boolean> =>
			(await page.getByTestId(testId).count()) > 0;

		// image.create: the page of the list; canvas.resize: the canvas fields
		await page.goto("/edit-image");
		await expect(page.getByTestId("image-new-form")).toBeVisible();
		await page.goto(`/edit-image/${id}`);
		await expect.poll(() => present("image-canvas-width")).toBe(true);
		// layer.add_*, group, ungroup, duplicate, remove, move (forward, back), align, fit, crop
		for (const testId of [
			"image-add-text",
			"image-add-rect",
			"image-add-fill",
			"image-add-image",
			"image-group",
			"image-ungroup",
			"image-duplicate",
			"image-remove",
			"image-forward",
			"image-back",
			"image-export-open",
		]) {
			expect(await present(testId), testId).toBe(true);
		}
		await row(page, "Photo").click();
		for (const testId of [
			"image-align-left",
			"image-fit-cover",
			"image-crop-reset",
			"image-field-asset",
			"image-field-opacity",
			"image-field-x",
		]) {
			expect(await present(testId), testId).toBe(true);
		}
		await row(page, "Title").click();
		for (const testId of [
			"image-field-text",
			"image-field-size",
			"image-field-color",
		]) {
			expect(await present(testId), testId).toBe(true);
		}
		await row(page, "Box").click();
		for (const testId of [
			"image-field-shape",
			"image-field-fill",
			"image-field-radius",
		]) {
			expect(await present(testId), testId).toBe(true);
		}
	});
});
