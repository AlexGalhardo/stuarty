import type { RenderWorkerLike } from "./client";
import type { PixelSource } from "./protocol";

// The render worker is a same-origin module script, so the CSP needs no
// `blob:` and no inline worker (SDD-007 section 3.6). null where the browser
// lacks what the worker needs (see support.ts).
export function createRenderWorker(): RenderWorkerLike<
	PixelSource,
	ImageBitmap
> | null {
	if (typeof Worker === "undefined") return null;
	return new Worker(new URL("./renderWorker.ts", import.meta.url), {
		type: "module",
	}) as unknown as RenderWorkerLike<PixelSource, ImageBitmap>;
}
