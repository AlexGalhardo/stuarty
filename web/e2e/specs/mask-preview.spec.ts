import type { Page } from "@playwright/test";
import { caller, expect, seedProject, test } from "./helpers";

type Rgb = [number, number, number];

// The colour of the canvas buffer at a fraction of its width and height.
async function sampleAt(
	page: Page,
	points: [number, number][],
): Promise<Rgb[]> {
	const canvas = page.getByRole("main", { name: "Preview" }).locator("canvas");
	await expect(canvas).toHaveAttribute("data-drawn", "0", { timeout: 20_000 });
	return canvas.evaluate(async (el, at) => {
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
		return at.map(([fx, fy]) => {
			const d = ctx.getImageData(
				Math.floor(fx * copy.width),
				Math.floor(fy * copy.height),
				1,
				1,
			).data;
			return [d[0] ?? 0, d[1] ?? 0, d[2] ?? 0] as Rgb;
		});
	}, points);
}

const distance = (a: Rgb, b: Rgb): number =>
	Math.abs(a[0] - b[0]) + Math.abs(a[1] - b[1]) + Math.abs(a[2] - b[2]);

// VV-015 (SDD-012 section 11.3, SDD-004 section 3.9): a mask in the plan hides
// what the preview draws outside it, and `invert` keeps the outside instead.
test.describe("mask-preview", () => {
	test("the preview draws the clip only inside the mask, and the inverse outside", async ({
		page,
		request,
	}) => {
		const seeded = await seedProject(request, {
			tracks: 1,
			perTrack: 1,
			clipDuration: "4s",
		});
		const itemId = seeded.itemIds[0] ?? "";
		const call = await caller(request);
		const setMask = async (mask: unknown): Promise<void> => {
			const head = (await call("GET", `/projects/${seeded.id}`)) as {
				revision: number;
			};
			await call("POST", `/projects/${seeded.id}/edit`, {
				base_revision: head.revision,
				operations: [{ op: "clip.set_mask", args: { item_id: itemId, mask } }],
			});
		};
		const points: [number, number][] = [
			[0.25, 0.5],
			[0.75, 0.5],
		];
		const half = {
			shape: "rect",
			x: 0,
			y: 0,
			width: 0.5,
			height: 1,
			feather: 0,
		};

		await page.goto(`/editor/${seeded.id}`);
		const [plainLeft, plainRight] = await sampleAt(page, points);
		expect(plainLeft).toBeDefined();
		expect(plainRight).toBeDefined();

		await setMask(half);
		await page.reload();
		const [left, right] = await sampleAt(page, points);
		expect(distance(left as Rgb, plainLeft as Rgb)).toBeLessThan(12);
		// Where the clip is hidden the canvas shows the project background.
		expect(distance(right as Rgb, plainRight as Rgb)).toBeGreaterThan(30);

		await setMask({ ...half, invert: true });
		await page.reload();
		const [invLeft, invRight] = await sampleAt(page, points);
		expect(distance(invLeft as Rgb, plainLeft as Rgb)).toBeGreaterThan(30);
		expect(distance(invRight as Rgb, plainRight as Rgb)).toBeLessThan(12);
	});
});
