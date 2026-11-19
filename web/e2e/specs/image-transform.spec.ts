import type { Page } from "@playwright/test";

import { expect, test } from "./helpers";
import { editImage, imageState, layerByName, newImage } from "./image-support";

// Epic 28 task 3: select, move, scale and rotate on the canvas, with snapping, by pointer and by keyboard. The
// server decides every result; the spec reads what it stored.

async function open(
	page: Page,
	id: string,
): Promise<{
	toScreen(x: number, y: number): Promise<{ x: number; y: number }>;
	zoom(): Promise<number>;
}> {
	await page.goto(`/edit-image/${id}`);
	await expect(page.getByTestId("image-frame")).toBeVisible({
		timeout: 30_000,
	});
	const stage = page.getByTestId("image-stage");
	const zoom = async (): Promise<number> =>
		Number(await stage.getAttribute("data-zoom"));
	return {
		zoom,
		toScreen: async (x, y) => {
			const box = await page.getByTestId("image-picture").boundingBox();
			if (!box) throw new Error("no picture");
			const z = await zoom();
			return { x: box.x + x * z, y: box.y + y * z };
		},
	};
}

async function seedBox(request: Parameters<typeof newImage>[0]): Promise<{
	id: string;
	call: Awaited<ReturnType<typeof newImage>>["call"];
	layer: () => Promise<NonNullable<ReturnType<typeof layerByName>>>;
}> {
	const { id, call } = await newImage(request, { name: "Transform spec" });
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
	]);
	const layer = async () => {
		const state = await imageState(call, id);
		const found = layerByName(state.layers, "Box");
		if (!found) throw new Error("no Box");
		return found;
	};
	return { id, call, layer };
}

const revisionOf = async (
	call: Awaited<ReturnType<typeof newImage>>["call"],
	id: string,
): Promise<number> => (await imageState(call, id)).revision;

test.describe("image transform", () => {
	test("a click selects the layer under the pointer and a click on nothing clears it", async ({
		page,
		request,
	}) => {
		const { id, layer } = await seedBox(request);
		const view = await open(page, id);
		const box = await layer();

		// the fill is under the shape: a click on the shape picks the shape
		const on = await view.toScreen(box.x + 20, box.y + 20);
		await page.mouse.click(on.x, on.y);
		const selection = page.getByTestId("image-selection");
		await expect(selection).toHaveAttribute("data-layer", box.id);
		await expect(
			page.getByRole("status").filter({ hasText: "Layer Box" }),
		).toBeVisible();

		await page.keyboard.up("Shift");
		const off = await view.toScreen(50, 50);
		await page.mouse.click(off.x, off.y);
		await expect(selection).toHaveCount(1);
		await expect(selection).not.toHaveAttribute("data-layer", box.id);
	});

	test("a drag moves the layer by the distance of the pointer and sends one command", async ({
		page,
		request,
	}) => {
		const { id, call, layer } = await seedBox(request);
		const view = await open(page, id);
		const before = await layer();
		const startRevision = await revisionOf(call, id);
		const z = await view.zoom();

		const from = await view.toScreen(before.x + 20, before.y + 20);
		await page.keyboard.down("Alt");
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + 60, from.y + 30, { steps: 5 });
		await page.mouse.up();
		await page.keyboard.up("Alt");

		await expect
			.poll(async () => (await layer()).x)
			.toBe(Math.round(before.x + 60 / z));
		const after = await layer();
		expect(after.y).toBe(Math.round(before.y + 30 / z));
		// one batch: one revision
		expect(await revisionOf(call, id)).toBe(startRevision + 1);
		// and the screen shows the server's answer
		await expect(page.getByTestId("image-frame")).toHaveAttribute(
			"data-revision",
			String(startRevision + 1),
		);
	});

	test("the edges snap to the canvas edge and a guide is drawn while it does", async ({
		page,
		request,
	}) => {
		const { id, layer } = await seedBox(request);
		const view = await open(page, id);
		const before = await layer();
		const z = await view.zoom();

		// drag the layer so that its left edge ends 3 screen pixels short of the canvas edge
		const from = await view.toScreen(before.x + 20, before.y + 20);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x - before.x * z + 3, from.y, { steps: 6 });
		await expect(page.getByTestId("image-guide-x")).toBeVisible();
		await page.mouse.up();
		await expect.poll(async () => (await layer()).x).toBe(0);
	});

	test("Escape cancels a drag and nothing is sent", async ({
		page,
		request,
	}) => {
		const { id, call, layer } = await seedBox(request);
		const view = await open(page, id);
		const before = await layer();
		const startRevision = await revisionOf(call, id);

		const from = await view.toScreen(before.x + 20, before.y + 20);
		await page.mouse.move(from.x, from.y);
		await page.mouse.down();
		await page.mouse.move(from.x + 80, from.y + 40, { steps: 4 });
		await page.keyboard.press("Escape");
		await page.mouse.up();
		// The cancelled drag sent nothing: the next command, an arrow key, is the very next revision.
		await page.getByTestId("image-stage").focus();
		await page.keyboard.press("ArrowRight");
		await expect.poll(() => revisionOf(call, id)).toBe(startRevision + 1);
		expect((await layer()).x).toBe(before.x + 1);
	});

	test("a corner handle scales the layer, keeping the opposite corner", async ({
		page,
		request,
	}) => {
		const { id, layer } = await seedBox(request);
		const view = await open(page, id);
		const before = await layer();
		const z = await view.zoom();

		const on = await view.toScreen(before.x + 20, before.y + 20);
		await page.mouse.click(on.x, on.y);
		const handle = page.locator('[data-handle="se"]').first();
		await expect(handle).toBeVisible();
		const at = await handle.boundingBox();
		if (!at) throw new Error("no handle");
		const cx = at.x + at.width / 2;
		const cy = at.y + at.height / 2;
		await page.keyboard.down("Alt");
		await page.mouse.move(cx, cy);
		await page.mouse.down();
		await page.mouse.move(cx + 40, cy + 20, { steps: 4 });
		await page.mouse.up();
		await page.keyboard.up("Alt");

		await expect
			.poll(async () => (await layer()).w)
			.toBe(Math.round(before.w + 40 / z));
		const after = await layer();
		expect(after.h).toBe(Math.round(before.h + 20 / z));
		expect(after.x).toBe(before.x);
		expect(after.y).toBe(before.y);
	});

	test("the rotate handle turns the layer", async ({ page, request }) => {
		const { id, layer } = await seedBox(request);
		const view = await open(page, id);
		const before = await layer();

		const on = await view.toScreen(before.x + 20, before.y + 20);
		await page.mouse.click(on.x, on.y);
		const handle = page.locator('[data-handle="rotate"]').first();
		await expect(handle).toBeVisible();
		const at = await handle.boundingBox();
		if (!at) throw new Error("no handle");
		// pull the handle to the right of the centre: a quarter turn clockwise
		const centre = await view.toScreen(
			before.x + before.w / 2,
			before.y + before.h / 2,
		);
		await page.mouse.move(at.x + at.width / 2, at.y + at.height / 2);
		await page.mouse.down();
		await page.mouse.move(centre.x + 150, centre.y, { steps: 6 });
		await page.mouse.up();

		await expect.poll(async () => (await layer()).rotation ?? 0).toBe(90);
	});

	test("arrow keys nudge by 1 pixel and by 10 with Shift, and Delete removes", async ({
		page,
		request,
	}) => {
		const { id, call, layer } = await seedBox(request);
		const view = await open(page, id);
		const before = await layer();

		const on = await view.toScreen(before.x + 20, before.y + 20);
		await page.mouse.click(on.x, on.y);
		await expect(page.getByTestId("image-selection")).toHaveCount(1);
		await page.keyboard.press("ArrowRight");
		await expect.poll(async () => (await layer()).x).toBe(before.x + 1);
		await page.keyboard.press("Shift+ArrowDown");
		await expect.poll(async () => (await layer()).y).toBe(before.y + 10);
		await page.keyboard.press("Delete");
		await expect
			.poll(
				async () =>
					layerByName((await imageState(call, id)).layers, "Box") === undefined,
			)
			.toBe(true);
	});
});
