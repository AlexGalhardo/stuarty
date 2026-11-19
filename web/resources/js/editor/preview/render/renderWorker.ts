import { cutRows, evenAround, whole } from "./pixels";
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

async function readSource(
	source: PixelSource,
	rect: Rect | undefined,
): Promise<Pixels> {
	if (typeof VideoFrame !== "undefined" && source instanceof VideoFrame) {
		const frameWidth = source.visibleRect?.width ?? source.codedWidth;
		const frameHeight = source.visibleRect?.height ?? source.codedHeight;
		const area = rect
			? whole(rect)
			: { x: 0, y: 0, w: frameWidth, h: frameHeight };
		const outer = rect ? evenAround(area, frameWidth, frameHeight) : area;
		const copy = new Uint8Array(outer.w * outer.h * 4);
		// The browser converts from the colour tags of the proxy (SDD-013 5.3.6).
		await source.copyTo(copy, {
			format: "RGBA",
			rect: { x: outer.x, y: outer.y, width: outer.w, height: outer.h },
			colorSpace: "srgb",
		});
		return { width: area.w, height: area.h, rgba: cutRows(copy, outer, area) };
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
