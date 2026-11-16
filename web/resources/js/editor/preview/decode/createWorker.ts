import type { WorkerLike } from "./client";

// The decode worker is a same-origin module script, so the CSP needs no
// `blob:` and no inline worker (SDD-007 section 3.6). null where the browser
// has no WebCodecs: every asset is then drawn by <video>.
export function createDecodeWorker(): WorkerLike<VideoFrame> | null {
	if (
		typeof Worker === "undefined" ||
		typeof VideoDecoder === "undefined" ||
		typeof EncodedVideoChunk === "undefined"
	) {
		return null;
	}
	return new Worker(new URL("./decodeWorker.ts", import.meta.url), {
		type: "module",
	}) as unknown as WorkerLike<VideoFrame>;
}
