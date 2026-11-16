import type { FrameLike } from "./decodeCore";
import { type CachedFrame, FrameCache, FrameLedger } from "./frameCache";
import type { FromWorker, ToWorker } from "./protocol";

// The setting that forces one path, for the end-to-end specs: `?decoder=video`
// keeps the pooled `<video>` elements, `?decoder=webcodecs` is the default
// spelled out (an asset whose config the browser refuses still falls back).
// Without it, WebCodecs is used where the browser has it (SDD-013 section 5.3,
// SDD-012 section 11.2).
export const DECODER_PARAM = "decoder";
export type DecoderPreference = "auto" | "webcodecs" | "video";

export function decoderPreference(search: string): DecoderPreference {
	const value = new URLSearchParams(search).get(DECODER_PARAM);
	return value === "webcodecs" || value === "video" ? value : "auto";
}

export interface WorkerLike<F> {
	postMessage(message: ToWorker): void;
	onmessage: ((event: { data: FromWorker<F> }) => void) | null;
	terminate(): void;
}

export interface ClientOptions<F extends FrameLike> {
	preference: DecoderPreference;
	// null when the browser has no WebCodecs or no Worker.
	createWorker(): WorkerLike<F> | null;
	indexUrl(asset: string): string;
	proxyUrl(asset: string): string;
	// A frame arrived or an asset fell back: the preview draws again.
	onChange(): void;
	now(): number;
	cacheSize?: number;
	idleMs?: number;
}

export type RequestMode = "seek" | "play";

export type FrameRequest<F> =
	| { state: "frame"; frame: F; exact: boolean; timestamp: number }
	| { state: "pending" }
	| { state: "fallback" };

interface AssetState {
	status: "opening" | "ready" | "fallback";
	inflight: number | null;
	// The latest source time asked for, sent when the reply of the request in
	// flight arrives and is not for it.
	wanted: { micros: number; mode: RequestMode } | null;
	lastUsed: number;
}

export const CACHE_FRAMES = 12;
export const IDLE_MS = 5000;

// The main-thread side of the decode worker: the frames it returns are cached
// by source frame, closed when evicted, and counted. It keeps one request per
// asset in flight and only the latest time asked for, so a scrub never queues.
export class DecodeClient<F extends FrameLike> {
	private worker: WorkerLike<F> | null = null;
	private readonly assets = new Map<string, AssetState>();
	private readonly ledger = new FrameLedger<F>();
	private readonly cache: FrameCache<F>;
	private nextId = 1;
	private workerOpen = 0;
	private peak = 0;
	private fallbacks = 0;
	private readonly idleMs: number;

	constructor(private readonly options: ClientOptions<F>) {
		this.cache = new FrameCache(options.cacheSize ?? CACHE_FRAMES, this.ledger);
		this.idleMs = options.idleMs ?? IDLE_MS;
	}

	// False when this client never decodes: every asset is drawn by `<video>`.
	get enabled(): boolean {
		return this.options.preference !== "video";
	}

	get stats(): {
		openFrames: number;
		peakOpenFrames: number;
		fallbackAssets: number;
	} {
		return {
			openFrames: this.ledger.count + this.workerOpen,
			peakOpenFrames: this.peak,
			fallbackAssets: this.fallbacks,
		};
	}

	private ensureWorker(): WorkerLike<F> | null {
		if (this.worker !== null) return this.worker;
		if (!this.enabled) return null;
		const worker = this.options.createWorker();
		if (worker === null) return null;
		worker.onmessage = (event) => this.receive(event.data);
		this.worker = worker;
		return worker;
	}

	// The frame of `asset` at a source time. `exact` is false when it is the
	// frame shown until the right one arrives. `ahead` is one frame in
	// microseconds: during playback the next frame is asked for too.
	request(
		asset: string,
		micros: number,
		mode: RequestMode,
		ahead = 0,
	): FrameRequest<F> {
		const worker = this.ensureWorker();
		if (worker === null) return { state: "fallback" };
		let state = this.assets.get(asset);
		if (state === undefined) {
			state = { status: "opening", inflight: null, wanted: null, lastUsed: 0 };
			this.assets.set(asset, state);
			worker.postMessage({
				type: "open",
				asset,
				indexUrl: this.options.indexUrl(asset),
				proxyUrl: this.options.proxyUrl(asset),
			});
		}
		state.lastUsed = this.options.now();
		if (state.status === "fallback") return { state: "fallback" };
		if (state.status === "opening") return { state: "pending" };

		const hit = this.cache.find(asset, micros);
		if (hit === undefined) {
			this.ask(asset, state, micros, mode);
		} else if (
			mode === "play" &&
			ahead > 0 &&
			this.cache.find(asset, micros + ahead) === undefined
		) {
			this.ask(asset, state, micros + ahead, mode);
		}
		if (hit !== undefined) return this.shown(hit, true);
		const stale = this.cache.latest(asset);
		return stale === undefined
			? { state: "pending" }
			: this.shown(stale, false);
	}

	private shown(entry: CachedFrame<F>, exact: boolean): FrameRequest<F> {
		return {
			state: "frame",
			frame: entry.frame,
			exact,
			timestamp: entry.frame.timestamp,
		};
	}

	private ask(
		asset: string,
		state: AssetState,
		micros: number,
		mode: RequestMode,
	): void {
		state.wanted = { micros, mode };
		if (state.inflight === null) this.send(asset, state);
	}

	private send(asset: string, state: AssetState): void {
		const wanted = state.wanted;
		if (wanted === null || this.worker === null) return;
		state.wanted = null;
		state.inflight = this.nextId++;
		this.worker.postMessage({
			type: "frame",
			id: state.inflight,
			asset,
			micros: wanted.micros,
			mode: wanted.mode,
		});
	}

	private receive(message: FromWorker<F>): void {
		this.workerOpen = message.open;
		const state = this.assets.get(message.asset);
		switch (message.type) {
			case "opened":
				if (state !== undefined) state.status = "ready";
				this.options.onChange();
				return;
			case "fallback":
				if (state !== undefined) {
					if (state.status !== "fallback") this.fallbacks++;
					state.status = "fallback";
					state.inflight = null;
					state.wanted = null;
				}
				this.cache.dropAsset(message.asset);
				this.options.onChange();
				return;
			case "skipped":
				if (state !== undefined) {
					state.inflight = null;
					this.send(message.asset, state);
				}
				return;
			case "frame": {
				// A frame of an asset dropped meanwhile is closed at once.
				if (state === undefined || state.status !== "ready") {
					message.frame.close();
					return;
				}
				this.ledger.adopt(message.frame);
				this.cache.put({
					asset: message.asset,
					sample: message.sample,
					lo: message.lo,
					hi: message.hi,
					frame: message.frame,
				});
				this.peak = Math.max(this.peak, this.stats.openFrames);
				state.inflight = null;
				this.send(message.asset, state);
				this.options.onChange();
			}
		}
	}

	// Run after a render: closes the frames evicted from the cache and releases
	// the assets that left the window `idleMs` ago.
	sweep(): void {
		const now = this.options.now();
		for (const [asset, state] of this.assets) {
			if (state.status === "fallback" || now - state.lastUsed < this.idleMs) {
				continue;
			}
			this.worker?.postMessage({ type: "drop", asset });
			this.cache.dropAsset(asset);
			this.assets.delete(asset);
		}
		this.cache.sweep();
	}

	// Whether the worker decodes `asset` or will once it has opened.
	handles(asset: string): boolean {
		const state = this.assets.get(asset);
		return state === undefined ? this.enabled : state.status !== "fallback";
	}

	dispose(): void {
		this.worker?.terminate();
		this.worker = null;
		this.cache.clear();
		this.ledger.releaseAll();
		this.assets.clear();
	}
}
