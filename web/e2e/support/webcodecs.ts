import type { Page } from "@playwright/test";
import type { Pixels } from "./ssim";

// Shared by the WebCodecs specs. P5-UI-002 (SDD-013 section 5.5, SDD-012 section 11.2 item 8): the preview
// decodes the VP9 proxy with WebCodecs in a worker. The canvas says what it
// drew: `data-drawn` the frame, `data-frame-us` the timestamp of the decoded
// sample, `data-peak-frames` the most VideoFrames that were open at once and
// `data-fallback-assets` the assets drawn by <video> instead.
//
// Chromium, Firefox and WebKit run it where the browser has WebCodecs and can
// decode the proxy's VP9; elsewhere the test skips and names the browser.
export const SEEKS = 500;
// The cache of the client (12), what the worker holds ahead (4) and the frame
// in transit; SDD-012 section 11.2 item 8.
export const OPEN_FRAMES_BOUND = 12 + 4 + 1;
export const MIN_SSIM = 0.97;

export const canvasOf = (page: Page) =>
	page.getByRole("main", { name: "Preview" }).locator("canvas");

// A small seeded generator, so a failing run can be repeated.
export function random(seed: number): () => number {
	let s = seed >>> 0;
	return () => {
		s = (Math.imul(s, 1664525) + 1013904223) >>> 0;
		return s / 2 ** 32;
	};
}

export async function skipWithoutWebCodecs(
	page: Page,
	skip: (condition: boolean, reason: string) => void,
	browserName: string,
): Promise<void> {
	await page.goto("/up");
	const supported = await page.evaluate(async () => {
		if (
			typeof VideoDecoder === "undefined" ||
			typeof EncodedVideoChunk === "undefined"
		) {
			return false;
		}
		try {
			const r = await VideoDecoder.isConfigSupported({
				codec: "vp09.00.10.08",
			});
			return r.supported === true;
		} catch {
			return false;
		}
	});
	skip(
		!supported,
		`${browserName} has no WebCodecs VP9 decoder here, so the preview decodes with <video>`,
	);
}

export interface Index {
	timestamps_us: number[];
	keyframes: number[];
}

export async function chunkIndex(
	page: Page,
	projectId: string,
	assetId: string,
): Promise<Index> {
	return page.evaluate(
		async ({ p, a }) => {
			const r = await fetch(`/api/v1/projects/${p}/media/${a}/chunks`);
			if (!r.ok) throw new Error(`chunks ${r.status}`);
			return (await r.json()) as Index;
		},
		{ p: projectId, a: assetId },
	);
}

// Clicks the ruler at a fraction of its width and returns the frame it chose.
export async function seekByRuler(
	page: Page,
	fraction: number,
): Promise<number> {
	const ruler = page.getByRole("slider", { name: "Playhead" });
	const box = await ruler.boundingBox();
	if (!box) throw new Error("no ruler");
	await page.mouse.click(
		box.x + 20 + fraction * (box.width - 40),
		box.y + box.height / 2,
	);
	return Number(await ruler.getAttribute("aria-valuenow"));
}

export async function pixelsOfBoth(
	page: Page,
	projectId: string,
	frame: number,
): Promise<{ canvas: Pixels; exported: Pixels }> {
	const out = await canvasOf(page).evaluate(
		async (el, { p, at }) => {
			const source = el as HTMLCanvasElement;
			const w = source.width;
			let response: Response;
			const giveUp = Date.now() + 60_000;
			for (;;) {
				response = await fetch(
					`/api/v1/projects/${p}/frame?at=${at}f&width=${w}`,
					{ headers: { Accept: "application/json" } },
				);
				if (response.status !== 429 || Date.now() > giveUp) break;
				await new Promise((r) => setTimeout(r, 200 + Math.random() * 600));
			}
			if (!response.ok) throw new Error(`frame ${response.status}`);
			const body = (await response.json()) as { image_base64: string };
			const bytes = Uint8Array.from(atob(body.image_base64), (c) =>
				c.charCodeAt(0),
			);
			const jpeg = await createImageBitmap(
				new Blob([bytes], { type: "image/jpeg" }),
			);
			const read = (
				image: CanvasImageSource,
			): { width: number; height: number; base64: string } => {
				const c = document.createElement("canvas");
				c.width = source.width;
				c.height = source.height;
				const g = c.getContext("2d");
				if (!g) throw new Error("no 2d context");
				g.drawImage(image, 0, 0, c.width, c.height);
				const data = g.getImageData(0, 0, c.width, c.height).data;
				let s = "";
				for (let i = 0; i < data.length; i += 0x8000) {
					s += String.fromCharCode(...data.subarray(i, i + 0x8000));
				}
				return { width: c.width, height: c.height, base64: btoa(s) };
			};
			return { canvas: read(source), exported: read(jpeg) };
		},
		{ p: projectId, at: frame },
	);
	const toPixels = (v: {
		width: number;
		height: number;
		base64: string;
	}): Pixels => ({
		width: v.width,
		height: v.height,
		data: new Uint8Array(Buffer.from(v.base64, "base64")),
	});
	return { canvas: toPixels(out.canvas), exported: toPixels(out.exported) };
}
