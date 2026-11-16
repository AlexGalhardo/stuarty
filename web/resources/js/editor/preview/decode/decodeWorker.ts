import {
	type ChunkIndex,
	parseChunkIndex,
	sampleAt,
	sampleWindow,
} from "./chunkIndex";
import { AssetDecoder, type DecodeDeps } from "./decodeCore";
import type { FromWorker, ToWorker } from "./protocol";

// The decode worker of SDD-013 section 5.3: one VideoDecoder per asset, fed
// from byte ranges of the proxy with the chunk index. It never reads the MP4
// boxes. A frame goes to the main thread by transfer, which detaches it here.
function readIndex(raw: unknown): ChunkIndex | null {
	const check = parseChunkIndex(raw);
	return check.ok ? check.index : null;
}

interface WorkerScope {
	postMessage(message: FromWorker, transfer?: Transferable[]): void;
	onmessage: ((event: MessageEvent<ToWorker>) => void) | null;
}

const scope = self as unknown as WorkerScope;
const assets = new Map<
	string,
	{ decoder: AssetDecoder<VideoFrame>; index: ChunkIndex }
>();
const opening = new Set<string>();

function open(): number {
	let held = 0;
	for (const { decoder } of assets.values()) held += decoder.openFrames;
	return held;
}

function post(message: FromWorker, transfer: Transferable[] = []): void {
	scope.postMessage(message, transfer);
}

function depsFor(proxyUrl: string): DecodeDeps<VideoFrame> {
	return {
		createDecoder: ({ output, error }) =>
			new VideoDecoder({ output, error: (e) => error(e) }),
		makeChunk: (init) => new EncodedVideoChunk(init),
		fetchRange: async (start, end) => {
			const response = await fetch(proxyUrl, {
				headers: { Range: `bytes=${start}-${end}` },
			});
			if (!response.ok) throw new Error(`proxy range: ${response.status}`);
			const body = new Uint8Array(await response.arrayBuffer());
			const range = /\/(\d+)$/.exec(
				response.headers.get("Content-Range") ?? "",
			);
			if (response.status === 206) {
				return { bytes: body, total: range ? Number(range[1]) : null };
			}
			// A server that ignored the Range answers the whole file.
			return { bytes: body.subarray(start, end + 1), total: body.length };
		},
	};
}

function fallback(asset: string, reason: string): void {
	assets.get(asset)?.decoder.close();
	assets.delete(asset);
	post({ type: "fallback", asset, reason, open: open() });
}

async function openAsset(
	asset: string,
	indexUrl: string,
	proxyUrl: string,
): Promise<void> {
	opening.add(asset);
	try {
		const response = await fetch(indexUrl);
		// 404: an asset ingested before the index existed, an audio or image
		// asset, or one with an alpha proxy (SDD-013 section 6.4).
		if (!response.ok) return fallback(asset, `index ${response.status}`);
		const index = readIndex(await response.json());
		if (index === null) return fallback(asset, "invalid index");
		const decoder = await AssetDecoder.open(
			index,
			depsFor(proxyUrl),
			async (config) =>
				(await VideoDecoder.isConfigSupported(config)).supported === true,
		);
		if (decoder === null) return fallback(asset, "config not supported");
		assets.set(asset, { decoder, index });
		post({ type: "opened", asset, open: open() });
	} catch (e) {
		fallback(asset, e instanceof Error ? e.message : "open failed");
	} finally {
		opening.delete(asset);
	}
}

async function serve(
	message: Extract<ToWorker, { type: "frame" }>,
): Promise<void> {
	const entry = assets.get(message.asset);
	if (entry === undefined) return;
	const sample = sampleAt(entry.index, message.micros / 1_000_000);
	try {
		const frame = await entry.decoder.frameAt(sample, message.mode);
		if (frame === null) {
			post({
				type: "skipped",
				id: message.id,
				asset: message.asset,
				open: open(),
			});
			return;
		}
		const { lo, hi } = sampleWindow(entry.index, sample);
		post(
			{
				type: "frame",
				id: message.id,
				asset: message.asset,
				sample,
				timestamp: frame.timestamp,
				lo,
				hi,
				frame,
				open: open(),
			},
			[frame],
		);
	} catch (e) {
		fallback(message.asset, e instanceof Error ? e.message : "decode failed");
	}
}

scope.onmessage = (event: MessageEvent<ToWorker>): void => {
	const message = event.data;
	switch (message.type) {
		case "open":
			if (!assets.has(message.asset) && !opening.has(message.asset)) {
				void openAsset(message.asset, message.indexUrl, message.proxyUrl);
			}
			break;
		case "frame":
			void serve(message);
			break;
		case "drop":
			assets.get(message.asset)?.decoder.close();
			assets.delete(message.asset);
			break;
	}
};
