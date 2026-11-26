import {
	cutRows,
	decodeVideoPlanes,
	evenAround,
	videoPlaneLayout,
	whole,
} from "./pixels";
import type {
	FromRenderWorker,
	PixelSource,
	Rect,
	ToRenderWorker,
} from "./protocol";
import { type Pixels, RenderService } from "./renderService";

// The render worker of SDD-013 section 4.7: the core module of the renderer
// runs here and draws on the CPU path, the reference the export also uses. It
// fetches the module once, checks its digest and answers `render` messages with
// the frame as an ImageBitmap.
interface WorkerScope {
	postMessage(message: FromRenderWorker, transfer: Transferable[]): void;
	onmessage: ((event: MessageEvent<ToRenderWorker>) => void) | null;
	close(): void;
}

const scope = self as unknown as WorkerScope;

function hex(bytes: ArrayBuffer): string {
	return [...new Uint8Array(bytes)]
		.map((b) => b.toString(16).padStart(2, "0"))
		.join("");
}

// The cut pixels at the size the layer is drawn at. The Go host has FFmpeg or its
// image scaler deliver an unrotated layer at its drawn size and the renderer then
// copies it; the effects that work on the layer's pixels (grain, a channel split)
// see that size, so a texture left at the source size draws them differently.
async function deliver(
	pixels: Pixels,
	size: [number, number] | undefined,
): Promise<Pixels> {
	if (size === undefined) return pixels;
	const [width, height] = size;
	if (width === pixels.width && height === pixels.height) return pixels;
	const from = new OffscreenCanvas(pixels.width, pixels.height);
	const fromContext = from.getContext("2d");
	if (fromContext === null) throw new Error("no 2d context");
	const straight = new Uint8ClampedArray(pixels.width * pixels.height * 4);
	straight.set(pixels.rgba);
	fromContext.putImageData(
		new ImageData(straight, pixels.width, pixels.height),
		0,
		0,
	);
	const scaled = await createImageBitmap(from, {
		resizeWidth: width,
		resizeHeight: height,
		resizeQuality: "high",
	});
	const to = new OffscreenCanvas(width, height);
	const context = to.getContext("2d", { willReadFrequently: true });
	if (context === null) {
		scaled.close();
		throw new Error("no 2d context");
	}
	if (scaled.width !== width || scaled.height !== height) {
		context.imageSmoothingEnabled = true;
		context.imageSmoothingQuality = "high";
		context.drawImage(scaled, 0, 0, width, height);
	} else {
		context.drawImage(scaled, 0, 0);
	}
	scaled.close();
	const data = context.getImageData(0, 0, width, height).data;
	return { width, height, rgba: new Uint8Array(data.buffer) };
}

async function readSource(
	source: PixelSource,
	rect: Rect | undefined,
	size: [number, number] | undefined,
): Promise<Pixels> {
	return deliver(await cutSource(source, rect), size);
}

async function cutSource(
	source: PixelSource,
	rect: Rect | undefined,
): Promise<Pixels> {
	if (typeof VideoFrame !== "undefined" && source instanceof VideoFrame) {
		const visible = source.visibleRect;
		const frameWidth = visible?.width ?? source.codedWidth;
		const frameHeight = visible?.height ?? source.codedHeight;
		const area = rect
			? whole(rect)
			: { x: 0, y: 0, w: frameWidth, h: frameHeight };
		const outer = rect ? evenAround(area, frameWidth, frameHeight) : area;
		const copy = new Uint8Array(outer.w * outer.h * 4);
		// The browser converts from the colour tags of the proxy (SDD-013 5.3.6).
		const layouts = await source.copyTo(copy, {
			format: "RGBA",
			rect: {
				x: (visible?.x ?? 0) + outer.x,
				y: (visible?.y ?? 0) + outer.y,
				width: outer.w,
				height: outer.h,
			},
			colorSpace: "srgb",
		});
		if (layouts.length === 1 && layouts[0].stride >= outer.w * 4) {
			return {
				width: area.w,
				height: area.h,
				rgba: cutRows(copy, outer, area),
			};
		}
		// WebKitGTK ignores options.format in VideoFrame.copyTo and writes native
		// YUV planes instead. Copy the full native planes and convert the crop.
		const layout = videoPlaneLayout(source.format, frameWidth, frameHeight);
		const options = layout ? { layout } : undefined;
		const full = new Uint8Array(source.allocationSize(options));
		const reported = await source.copyTo(full, options);
		return {
			width: area.w,
			height: area.h,
			rgba: decodeVideoPlanes(
				full,
				layout ?? reported,
				source.format,
				source.colorSpace,
				area,
			),
		};
	}
	const bitmap = source as unknown as ImageBitmap;
	const x = Math.round(rect?.x ?? 0);
	const y = Math.round(rect?.y ?? 0);
	const width = Math.round(rect?.w ?? bitmap.width);
	const height = Math.round(rect?.h ?? bitmap.height);
	const canvas = new OffscreenCanvas(width, height);
	const context = canvas.getContext("2d", { willReadFrequently: true });
	if (context === null) throw new Error("no 2d context");
	context.imageSmoothingEnabled = false;
	context.drawImage(bitmap, x, y, width, height, 0, 0, width, height);
	const data = context.getImageData(0, 0, width, height).data;
	return { width, height, rgba: new Uint8Array(data.buffer) };
}

const service = new RenderService<PixelSource, ImageBitmap>(
	{
		fetchModule: async (url) => {
			const response = await fetch(url, { credentials: "same-origin" });
			if (!response.ok) throw new Error(`module: HTTP ${response.status}`);
			return response.arrayBuffer();
		},
		sha256: async (bytes) => hex(await crypto.subtle.digest("SHA-256", bytes)),
		read: readSource,
		toBitmap: (rgba, width, height) =>
			createImageBitmap(new ImageData(rgba, width, height)),
	},
	{ post: (message, transfer) => scope.postMessage(message, transfer) },
);

scope.onmessage = (event) => {
	const message = event.data;
	void service.receive(message).then(() => {
		if (message.type === "dispose") scope.close();
	});
};
