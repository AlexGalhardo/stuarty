import { createHash } from "node:crypto";
import { readFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";

import { caller } from "./helpers";
import {
	expect,
	imageState,
	layerByName,
	seedPhoto,
	test,
} from "./image-support";

// Epic 28, acceptance criterion 1: the thumbnail of the reference scenario of epic 27 built with the screen
// only (the photo is one of the workspace library, as it is for an agent), and exported. It leaves the brand kit
// alone: that belongs to the workspace and is the subject of image-template-brand.spec.ts.
test.describe("image thumbnail by the screen", () => {
	test.describe.configure({ timeout: 180_000, mode: "serial" });

	test("a person makes a video thumbnail and exports it without leaving Stuarty", async ({
		page,
		request,
	}) => {
		await seedPhoto(request, 0);
		const call = await caller(request, "agent");

		// a new image of the video thumbnail preset, from the list
		await page.goto("/edit-image");
		await page.getByLabel("Name").first().fill("Episode 12 thumbnail");
		await page.getByRole("button", { name: "Create image" }).click();
		await expect(page).toHaveURL(/\/edit-image\/img_/);
		const id = page.url().split("/").pop() ?? "";
		await expect(page.getByTestId("image-frame")).toBeVisible({
			timeout: 60_000,
		});

		// background, photo covering the canvas, a title bar, the title, and a small mark in a corner
		await page.getByTestId("image-add-fill").click();
		await page.getByTestId("image-field-fill").fill("#203040");
		await page.getByTestId("image-field-fill").press("Enter");
		await page.getByTestId("image-add-image").click();
		await page.getByTestId("image-picker-item").first().click();
		await page.getByTestId("image-fit-cover").click();

		await page.getByTestId("image-add-rect").click();
		await page.getByTestId("image-field-fill").fill("#FFE600");
		await page.getByTestId("image-field-fill").press("Enter");
		await page.getByTestId("image-field-w").fill("1100");
		await page.getByTestId("image-field-w").press("Enter");
		await page.getByTestId("image-field-h").fill("180");
		await page.getByTestId("image-field-h").press("Enter");
		await page.getByTestId("image-align-center").click();
		await page.getByTestId("image-field-y").fill("70%");
		await page.getByTestId("image-field-y").press("Enter");

		await page.getByTestId("image-add-text").click();
		await page.getByTestId("image-field-text").fill("HOW IT WORKS");
		await page.getByTestId("image-field-text").blur();
		await page.getByTestId("image-field-size").fill("110");
		await page.getByTestId("image-field-size").press("Enter");
		await page.getByTestId("image-field-color").fill("#101820");
		await page.getByTestId("image-field-color").press("Enter");
		await page.getByTestId("image-field-w").fill("1000");
		await page.getByTestId("image-field-w").press("Enter");
		await page.getByTestId("image-align-center").click();
		await page.getByTestId("image-field-y").fill("72%");
		await page.getByTestId("image-field-y").press("Enter");

		await page.getByTestId("image-add-image").click();
		await page.getByTestId("image-picker-item").first().click();
		await page.getByTestId("image-field-w").fill("160");
		await page.getByTestId("image-field-w").press("Enter");
		await page.getByTestId("image-field-h").fill("90");
		await page.getByTestId("image-field-h").press("Enter");
		await page.getByTestId("image-field-x").fill("40");
		await page.getByTestId("image-field-x").press("Enter");
		await page.getByTestId("image-field-y").fill("40");
		await page.getByTestId("image-field-y").press("Enter");

		// one step undone and done again, as in the scenario
		await page.getByTestId("image-undo").click();
		await page.getByTestId("image-redo").click();

		await expect
			.poll(async () =>
				(await imageState(call, id)).layers.map((l) => l.kind).join(","),
			)
			.toBe("fill,image,shape,text,image");
		const state = await imageState(call, id);
		expect(layerByName(state.layers, "Text")?.text).toBe("HOW IT WORKS");
		expect(layerByName(state.layers, "Shape")?.fill).toBe("#FFE600");
		expect(layerByName(state.layers, "Text")?.style?.color).toBe("#101820");

		// the picture on screen is the revision made, and the export is the file of that revision
		await expect(page.getByTestId("image-frame")).toHaveAttribute(
			"data-revision",
			String(state.revision),
			{ timeout: 60_000 },
		);
		await page.getByTestId("image-export-open").click();
		await page.getByTestId("image-export-run").click();
		await expect(page.getByTestId("image-export-result")).toContainText(
			"1280 x 720",
		);
		const download = page.waitForEvent("download");
		await page.getByTestId("image-export-download").click();
		const file = await download;
		const saved = join(tmpdir(), `stuarty-e2e-thumb-${id}.png`);
		await file.saveAs(saved);
		const bytes = readFileSync(saved);
		expect(bytes.subarray(0, 8).toString("hex")).toBe("89504e470d0a1a0a");
		expect(createHash("sha256").update(bytes).digest("hex")).toHaveLength(64);
		expect(bytes.length).toBeGreaterThan(10_000);
	});
});
