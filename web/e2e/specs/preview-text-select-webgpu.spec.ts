import type { Page } from "@playwright/test";
import { canvasOf } from "../support/webcodecs";
import { afterRender, caller, expect, seedProject, test } from "./helpers";

// VV-222: an unhandled exception ("[BindGroup] no usable resource") came out of
// the WebGPU preview when a text item was selected. The preview draws a text item
// as a bitmap of the plan; selecting it redraws the frame and moves the handles.
// This guard selects a text item on the PixiJS executor forced onto WebGPU and
// fails on any uncaught exception of the page.
// The headless shell of Playwright gives no WebGPU adapter; the full Chromium does, on
// the software adapter too, so the guard runs on a machine with no GPU.
test.use({
	channel: "chromium",
	launchOptions: {
		args: [
			"--enable-unsafe-webgpu",
			"--use-webgpu-adapter=swiftshader",
			"--enable-unsafe-swiftshader",
		],
	},
});

test.describe("preview-text-select-webgpu @chromium-only", () => {
	async function hasAdapter(page: Page): Promise<boolean> {
		return page.evaluate(async () => {
			const gpu = (
				navigator as Navigator & {
					gpu?: { requestAdapter(): Promise<unknown> };
				}
			).gpu;
			if (gpu === undefined) return false;
			return (await gpu.requestAdapter()) !== null;
		});
	}

	test("selecting a text item raises no exception on the WebGPU preview", async ({
		page,
		request,
	}) => {
		const errors: string[] = [];
		page.on("pageerror", (e) => errors.push(`pageerror: ${e.message}`));
		page.on("console", (m) => {
			if (m.type() === "error" || m.text().includes("[BindGroup]")) {
				errors.push(`console.${m.type()}: ${m.text()}`);
			}
		});

		const seeded = await seedProject(request, { tracks: 1, perTrack: 0 });
		const call = await caller(request);
		const head = (await call("GET", `/projects/${seeded.id}`)) as {
			revision: number;
		};
		const out = (await call("POST", `/projects/${seeded.id}/edit`, {
			base_revision: head.revision,
			operations: [
				{
					op: "text.add",
					args: {
						track_id: seeded.trackIds[0],
						start: "0s",
						duration: "3s",
						text: "Hello WebGPU",
					},
				},
			],
		})) as { created: string[] };
		const itemId = out.created[0] as string;

		await page.goto(`/editor/${seeded.id}?executor=pixi&renderer=webgpu`);
		test.skip(!(await hasAdapter(page)), "no WebGPU adapter in this browser"); // verify:env-skip the browser has no WebGPU adapter

		const canvas = canvasOf(page);
		await expect(canvas).toHaveAttribute("data-executor", "pixi");
		// A canvas that holds a WebGPU context refuses a WebGL one: the guard really runs on WebGPU.
		await expect
			.poll(() =>
				canvas.evaluate(
					(c) =>
						(c as HTMLCanvasElement).getContext("webgpu") !== null &&
						(c as HTMLCanvasElement).getContext("webgl2") === null,
				),
			)
			.toBe(true);
		const item = page.locator(`[data-item-id="${itemId}"]`).first();
		await expect(item).toBeVisible();
		await item.click();
		await expect(item).toHaveAttribute("aria-selected", "true");
		// Draw again with the selection in place, then once more by a seek.
		await page.locator("body").press("ArrowRight");
		await afterRender(page);
		await afterRender(page);
		expect(errors).toEqual([]);
	});
});
