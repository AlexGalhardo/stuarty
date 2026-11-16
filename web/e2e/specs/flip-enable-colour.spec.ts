import type { Page } from "@playwright/test";
import { NO_VIDEO_DECODE_REASON, noVideoDecode } from "../support/browsers";
import { caller, expect, markerClip, seedProject, test } from "./helpers";

interface Posted {
	operations: { op: string; args: Record<string, unknown> }[];
}

function recordEdits(page: Page): Posted[] {
	const edits: Posted[] = [];
	page.on("request", (r) => {
		if (r.method() === "POST" && r.url().endsWith("/edit")) {
			edits.push(r.postDataJSON());
		}
	});
	return edits;
}

// The centre of the red marker of markerClip on the preview canvas, as a
// fraction of its width and height.
async function redCentre(page: Page): Promise<{ x: number; y: number }> {
	const canvas = page.getByRole("main", { name: "Preview" }).locator("canvas");
	await expect(canvas).toHaveAttribute("data-drawn", "0", { timeout: 20_000 });
	return canvas.evaluate(async (el) => {
		// The page may not fetch a data: URL (connect-src), so the PNG is decoded
		// from its bytes.
		const base64 =
			(el as HTMLCanvasElement).toDataURL("image/png").split(",")[1] ?? "";
		const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
		const bitmap = await createImageBitmap(
			new Blob([bytes], { type: "image/png" }),
		);
		const scratch = new OffscreenCanvas(bitmap.width, bitmap.height);
		const ctx = scratch.getContext("2d");
		if (ctx === null) throw new Error("no 2d context");
		ctx.drawImage(bitmap, 0, 0);
		const { data } = ctx.getImageData(0, 0, bitmap.width, bitmap.height);
		let sx = 0;
		let sy = 0;
		let n = 0;
		for (let i = 0; i < data.length; i += 4) {
			if (
				(data[i] ?? 0) > 180 &&
				(data[i + 1] ?? 0) < 80 &&
				(data[i + 2] ?? 0) < 80
			) {
				const p = i / 4;
				sx += p % bitmap.width;
				sy += Math.floor(p / bitmap.width);
				n++;
			}
		}
		return { x: sx / n / bitmap.width, y: sy / n / bitmap.height };
	});
}

// TASK-0273 (SDD-012 sections 9.3, 12.1 and 7.4, SDD-004 sections 3.5 and 3.10):
// flip, enable, white balance, hue and noise reduction.
test.describe("flip, enable, colour and noise reduction", () => {
	test.describe.configure({ timeout: 120_000 });

	test("the inspector sends one command per control with only the changed field", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const itemId = seeded.itemIds[0] ?? "";
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator("[data-item-id]").click();
		await expect(page.getByRole("heading", { name: /^Clip/ })).toBeVisible();
		const edits = recordEdits(page);

		await page.getByLabel("Flip horizontally").check();
		await expect.poll(() => edits.length).toBe(1);
		await page.getByLabel("Flip vertically").check();
		await expect.poll(() => edits.length).toBe(2);
		for (const [label, key] of [
			["Temperature", "End"],
			["Tint", "Home"],
			["Hue", "End"],
			["Noise reduction", "End"],
		] as const) {
			const slider = page.getByLabel(label, { exact: true });
			await slider.scrollIntoViewIfNeeded();
			await slider.focus();
			await slider.press(key);
		}
		await expect.poll(() => edits.length).toBe(6);
		await page.getByLabel("On", { exact: true }).uncheck();
		await expect.poll(() => edits.length).toBe(7);

		expect(edits.map((e) => e.operations)).toEqual([
			[{ op: "clip.set_flip", args: { item_id: itemId, horizontal: true } }],
			[{ op: "clip.set_flip", args: { item_id: itemId, vertical: true } }],
			[{ op: "clip.set_color", args: { item_id: itemId, temperature: 1 } }],
			[{ op: "clip.set_color", args: { item_id: itemId, tint: -1 } }],
			[{ op: "clip.set_color", args: { item_id: itemId, hue: 180 } }],
			[{ op: "item.set_audio", args: { item_id: itemId, denoise: 1 } }],
			[{ op: "item.set_enabled", args: { item_id: itemId, enabled: false } }],
		]);
	});

	test("V turns the selected item off and on; it is dimmed with a badge", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const itemId = seeded.itemIds[0] ?? "";
		await page.goto(`/editor/${seeded.id}?debug=1`);
		const item = page.locator(`[data-item-id="${itemId}"]`);
		await item.click();
		const edits = recordEdits(page);

		await page.keyboard.press("v");
		await expect.poll(() => edits.length).toBe(1);
		await expect(item).toHaveAttribute("data-disabled", "");
		await expect(item.locator('[data-badge="disabled"]')).toBeVisible();
		await expect(page.getByLabel("On", { exact: true })).not.toBeChecked();

		await page.keyboard.press("v");
		await expect.poll(() => edits.length).toBe(2);
		await expect(item).not.toHaveAttribute("data-disabled", "");
		expect(edits.map((e) => e.operations)).toEqual([
			[{ op: "item.set_enabled", args: { item_id: itemId, enabled: false } }],
			[{ op: "item.set_enabled", args: { item_id: itemId, enabled: true } }],
		]);
	});

	test("the preview mirrors the picture inside the item", async ({
		page,
		request,
		browserName,
	}) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON);
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "5s",
			files: [markerClip(640, 360)],
		});
		await page.goto(`/editor/${seeded.id}`);
		const plain = await redCentre(page);

		const call = await caller(request);
		const edit = async (operations: unknown[]): Promise<void> => {
			const head = (await call("GET", `/projects/${seeded.id}`)) as {
				revision: number;
			};
			await call("POST", `/projects/${seeded.id}/edit`, {
				base_revision: head.revision,
				operations,
			});
		};
		const itemId = seeded.itemIds[0];
		await edit([
			{
				op: "clip.set_flip",
				args: { item_id: itemId, horizontal: true },
			},
		]);
		await page.reload();
		const mirrored = await redCentre(page);
		expect(mirrored.x).toBeCloseTo(1 - plain.x, 1);
		expect(mirrored.y).toBeCloseTo(plain.y, 1);

		await edit([
			{
				op: "clip.set_flip",
				args: { item_id: itemId, horizontal: false, vertical: true },
			},
		]);
		await page.reload();
		const upsideDown = await redCentre(page);
		expect(upsideDown.x).toBeCloseTo(plain.x, 1);
		expect(upsideDown.y).toBeCloseTo(1 - plain.y, 1);
	});
});
