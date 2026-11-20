import { mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import type { APIRequestContext, Page } from "@playwright/test";

import { caller, seedProject } from "./helpers";
import {
	editImage,
	expect,
	type ImageLayer,
	imageState,
	layerByName,
	newImage,
	test,
} from "./image-support";

// Epic 49 task 6: the colour lookup table (.cube) of a layer in the inspector (SDD-021 section 3.1). The control
// sends layer.adjust with `lut`; the picture the window shows is the server's render, so the spec reads a pixel of it.

// A 2 point table that inverts the colours: the expected pixels are easy to state.
const INVERT_CUBE = `TITLE "invert"
LUT_3D_SIZE 2
1 1 1
0 1 1
1 0 1
0 0 1
1 1 0
0 1 0
1 0 0
0 0 0
`;

async function seedLut(
	request: APIRequestContext,
): Promise<{ hash: string; name: string }> {
	const dir = join(tmpdir(), "stuarty-e2e-media");
	mkdirSync(dir, { recursive: true });
	const file = join(dir, "invert-grade.cube");
	writeFileSync(file, INVERT_CUBE);
	// A table reaches the workspace as it does for a video: imported into a project (SDD-011 section 7.1).
	await seedProject(request, { files: [file], tracks: 0, perTrack: 0 });
	const call = await caller(request);
	const library = (await call("GET", "/assets?kind=lut")) as unknown as {
		assets: { hash: string; name: string }[];
	};
	const found = library.assets.find((a) =>
		a.name.endsWith("invert-grade.cube"),
	);
	if (!found) throw new Error("the table is not in the library");
	return found;
}

const row = (page: Page, name: string) =>
	page.getByRole("treeitem", { name: new RegExp(`^${name},`) });

// pixelOf reads a pixel of the picture the window shows, which is the render of the server.
async function pixelOf(page: Page): Promise<number[]> {
	return page.getByTestId("image-frame").evaluate((img: HTMLImageElement) => {
		const canvas = document.createElement("canvas");
		canvas.width = img.naturalWidth;
		canvas.height = img.naturalHeight;
		const ctx = canvas.getContext("2d");
		if (ctx === null) throw new Error("no 2d context");
		ctx.drawImage(img, 0, 0);
		return Array.from(ctx.getImageData(10, 10, 1, 1).data);
	});
}

function near(actual: number[], want: number[]): void {
	for (let i = 0; i < want.length; i++) {
		expect(Math.abs((actual[i] ?? 0) - (want[i] ?? 0))).toBeLessThanOrEqual(2);
	}
}

test.describe("image lookup table", () => {
	test.describe.configure({ timeout: 120_000 });

	test("choosing a table grades the picture the server draws, and the strength and the removal follow", async ({
		page,
		request,
	}) => {
		const lut = await seedLut(request);
		const { id, call } = await newImage(request, { name: "LUT spec" });
		await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#203040", name: "Back" } },
		]);
		const layer = async (): Promise<ImageLayer> => {
			const found = layerByName((await imageState(call, id)).layers, "Back");
			if (!found) throw new Error("no layer Back");
			return found;
		};
		const shown = async (): Promise<void> => {
			const state = await imageState(call, id);
			await expect(page.getByTestId("image-frame")).toHaveAttribute(
				"data-revision",
				String(state.revision),
			);
		};

		await page.goto(`/edit-image/${id}`);
		await row(page, "Back").click();
		await expect(page.getByTestId("image-adjust-fields")).toBeVisible();
		await shown();
		near(await pixelOf(page), [32, 48, 64, 255]);

		// the intensity has nothing to change until a table is chosen
		await expect(page.getByTestId("image-adjust-lut-intensity")).toBeDisabled();

		await page
			.getByTestId("image-adjust-lut")
			.selectOption({ label: lut.name });
		await expect
			.poll(async () => (await layer()).adjust)
			.toEqual({ lut: { asset: lut.hash, intensity: 100 } });
		await shown();
		near(await pixelOf(page), [223, 207, 191, 255]);

		const input = page.getByTestId("image-adjust-lut-intensity");
		await input.fill("50");
		await input.press("Enter");
		await expect
			.poll(async () => (await layer()).adjust)
			.toEqual({ lut: { asset: lut.hash, intensity: 50 } });
		await shown();
		near(await pixelOf(page), [128, 128, 128, 255]);

		await page.getByTestId("image-adjust-lut").selectOption({ index: 0 });
		await expect.poll(async () => (await layer()).adjust).toBeUndefined();
		await shown();
		near(await pixelOf(page), [32, 48, 64, 255]);
	});

	test("the same operation refuses a file that is not a table, and an unknown one", async ({
		request,
	}) => {
		const { id, call } = await newImage(request, { name: "LUT refusals" });
		const state = await editImage(call, id, [
			{ op: "layer.add_fill", args: { color: "#203040", name: "Back" } },
		]);
		const back = layerByName(state.layers, "Back");
		if (!back) throw new Error("no layer Back");
		const send = (asset: string) =>
			call("POST", `/images/${id}/edit`, {
				base_revision: state.revision,
				operations: [
					{ op: "layer.adjust", args: { layer_id: back.id, lut: { asset } } },
				],
			});

		await expect(send(`sha256:${"0".repeat(64)}`)).rejects.toThrow(
			/E_NOT_FOUND/,
		);
		await expect(send("sha256:not-a-hash")).rejects.toThrow(/E_VALIDATION/);

		const lut = await seedLut(request);
		const photo = (await call("GET", "/assets?kind=image")) as unknown as {
			assets: { hash: string }[];
		};
		const image = photo.assets[0];
		if (image) {
			await expect(send(image.hash)).rejects.toThrow(/E_TYPE_MISMATCH/);
		}
		await expect(send(lut.hash)).resolves.toBeTruthy();
	});
});
