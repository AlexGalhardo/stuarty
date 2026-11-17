import type { Page } from "@playwright/test";
import { NO_VIDEO_DECODE_REASON, noVideoDecode } from "../support/browsers";
import { expect, seedProject, test } from "./helpers";

type Rgb = [number, number, number];

async function sampleAt(page: Page, fx: number, fy: number): Promise<Rgb> {
	const canvas = page.getByRole("main", { name: "Preview" }).locator("canvas");
	await expect(canvas).toHaveAttribute("data-drawn", "0", { timeout: 20_000 });
	return canvas.evaluate(
		async (el, at) => {
			const source = el as HTMLCanvasElement;
			const copy = document.createElement("canvas");
			copy.width = source.width;
			copy.height = source.height;
			const ctx = copy.getContext("2d");
			if (ctx === null) throw new Error("no 2d context");
			const image = new Image();
			image.src = source.toDataURL("image/png");
			await image.decode();
			ctx.drawImage(image, 0, 0);
			const d = ctx.getImageData(
				Math.floor(at[0] * copy.width),
				Math.floor(at[1] * copy.height),
				1,
				1,
			).data;
			return [d[0] ?? 0, d[1] ?? 0, d[2] ?? 0] as Rgb;
		},
		[fx, fy],
	);
}

const toRgb = (hex: string): Rgb => [
	Number.parseInt(hex.slice(1, 3), 16),
	Number.parseInt(hex.slice(3, 5), 16),
	Number.parseInt(hex.slice(5, 7), 16),
];

// VV-186: the eyedropper reads the colour of the preview canvas under the
// click and sends it with `clip.set_chroma_key`.
test.describe("chroma key eyedropper", () => {
	test("a click on the preview sets the key colour to the pixel under it", async ({
		page,
		request,
		browserName,
	}) => {
		test.skip(noVideoDecode(browserName), NO_VIDEO_DECODE_REASON);
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const itemId = seeded.itemIds[0] as string;
		await page.goto(`/editor/${seeded.id}?debug=1`);
		await page.locator(`[data-item-id="${itemId}"]`).click();
		await page.getByRole("checkbox", { name: "Chroma key" }).check();
		const pick = page.getByRole("button", {
			name: "Pick the colour from the preview",
		});
		await pick.click();
		await expect(pick).toHaveAttribute("aria-pressed", "true");

		const overlay = page.getByTestId("canvas-overlay");
		const box = await overlay.boundingBox();
		if (box === null) throw new Error("no overlay");
		const at = [0.2, 0.3] as const;
		const expected = await sampleAt(page, at[0], at[1]);
		await overlay.click({
			position: { x: box.width * at[0], y: box.height * at[1] },
		});
		await expect(pick).toHaveAttribute("aria-pressed", "false");
		await expect
			.poll(async () =>
				page.evaluate((id) => {
					const doc = window.__vv?.document as {
						items: Record<string, { chroma_key?: { color: string } }>;
					};
					return doc.items[id]?.chroma_key?.color ?? "";
				}, itemId),
			)
			.not.toBe("#00FF00");
		const stored = await page.evaluate((id) => {
			const doc = window.__vv?.document as {
				items: Record<string, { chroma_key?: { color: string } }>;
			};
			return doc.items[id]?.chroma_key?.color ?? "";
		}, itemId);
		const got = toRgb(stored);
		for (let c = 0; c < 3; c++) {
			expect(
				Math.abs((got[c] as number) - (expected[c] as number)),
			).toBeLessThan(10);
		}
	});

	test("Esc cancels the pick without sending anything", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, { tracks: 1, perTrack: 1 });
		const itemId = seeded.itemIds[0] as string;
		await page.goto(`/editor/${seeded.id}`);
		await page.locator(`[data-item-id="${itemId}"]`).click();
		await page.getByRole("checkbox", { name: "Chroma key" }).check();
		const pick = page.getByRole("button", {
			name: "Pick the colour from the preview",
		});
		await pick.click();
		await expect(pick).toHaveAttribute("aria-pressed", "true");
		await page.keyboard.press("Escape");
		await expect(pick).toHaveAttribute("aria-pressed", "false");
	});
});
