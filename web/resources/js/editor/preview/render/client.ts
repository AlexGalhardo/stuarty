import type {
	FromRenderWorker,
	PixelSource,
	Rect,
	RenderBudgets,
	RenderInput,
	RenderReason,
	ToRenderWorker,
} from "./protocol";
import { RenderError } from "./renderCore";

export interface RenderWorkerLike<S, B> {
	postMessage(message: ToRenderWorker<S>, transfer?: Transferable[]): void;
	onmessage: ((event: { data: FromRenderWorker<B> }) => void) | null;
	terminate(): void;
}

export interface RenderFailure {
	seq?: number;
	reason: RenderReason;
	detail: string;
}

export interface RenderClientHooks<B> {
	// The frame of `seq` is drawn. The bitmap is the receiver's to close.
	onFrame(bitmap: B, seq: number): void;
	// A render or an upload failed. `missing_texture` names the item or key in
	// `detail`: the receiver uploads it and renders again (SDD-013 section 4.7).
	onError(failure: RenderFailure): void;
}

// The main-thread end of the render worker. At most one render is in flight;
// a newer request replaces the one waiting, so a scrub costs one frame of
// latency and not a queue (SDD-013 section 4.7 rules 1 and 2).
export class RenderClient<S extends PixelSource, B> {
	private seq = 0;
	private inflight: number | null = null;
	private waiting: { input: RenderInput; seq: number } | null = null;
	private ready: ((path: "cpu" | "gpu") => void) | null = null;
	private failed: ((failure: RenderError) => void) | null = null;
	private open = 0;
	private peak = 0;
	private path: "cpu" | "gpu" | null = null;
	private disposed = false;

	constructor(
		private readonly worker: RenderWorkerLike<S, B>,
		private readonly hooks: RenderClientHooks<B>,
		private readonly closeBitmap: (bitmap: B) => void,
	) {
		worker.onmessage = (event) => this.receive(event.data);
	}

	get chosenPath(): "cpu" | "gpu" | null {
		return this.path;
	}

	// Sources the worker still holds; its peak is what the leak check reads.
	get stats(): { workerOpen: number; workerPeak: number } {
		return { workerOpen: this.open, workerPeak: this.peak };
	}

	init(
		module: string,
		digest: string,
		prefer: "gpu" | "cpu",
		budgets?: RenderBudgets,
	): Promise<"cpu" | "gpu"> {
		return new Promise((resolve, reject) => {
			this.ready = resolve;
			this.failed = reject;
			this.worker.postMessage({
				type: "init",
				module,
				digest,
				prefer,
				budgets,
			});
		});
	}

	// The worker closes the source after copying; it is transferred, so this
	// side holds no reference afterwards.
	texture(id: string, frame: S, rect?: Rect): void {
		this.worker.postMessage({ type: "texture", id, frame, rect }, [
			frame as unknown as Transferable,
		]);
	}

	bitmap(
		key: string,
		bitmap: S,
		natural: [number, number],
		center: [number, number],
	): void {
		this.worker.postMessage({ type: "bitmap", key, bitmap, natural, center }, [
			bitmap as unknown as Transferable,
		]);
	}

	lut(id: string, size: number, data: Uint8Array): void {
		this.worker.postMessage({ type: "lut", id, size, data });
	}

	release(ids: string[]): void {
		if (ids.length > 0) this.worker.postMessage({ type: "release", ids });
	}

	// Asks for a frame. Returns the sequence number it will answer with.
	render(input: RenderInput): number {
		const seq = ++this.seq;
		if (this.inflight !== null) {
			this.waiting = { input, seq };
			return seq;
		}
		this.send(input, seq);
		return seq;
	}

	dispose(): void {
		if (this.disposed) return;
		this.disposed = true;
		this.waiting = null;
		this.worker.postMessage({ type: "dispose" });
		this.worker.terminate();
	}

	private send(input: RenderInput, seq: number): void {
		this.inflight = seq;
		this.worker.postMessage({ type: "render", seq, input });
	}

	private flush(): void {
		this.inflight = null;
		const next = this.waiting;
		this.waiting = null;
		if (next !== null && !this.disposed) this.send(next.input, next.seq);
	}

	private receive(message: FromRenderWorker<B>): void {
		this.open = message.open;
		this.peak = Math.max(this.peak, message.open);
		if (this.disposed) {
			if (message.type === "frame") this.closeBitmap(message.bitmap);
			return;
		}
		switch (message.type) {
			case "ready":
				this.path = message.path;
				this.ready?.(message.path);
				this.ready = null;
				this.failed = null;
				return;
			case "frame": {
				const stale = message.seq !== this.inflight;
				if (stale) {
					this.closeBitmap(message.bitmap);
					return;
				}
				this.hooks.onFrame(message.bitmap, message.seq);
				this.flush();
				return;
			}
			case "error": {
				const failure: RenderFailure = {
					seq: message.seq,
					reason: message.reason,
					detail: message.detail,
				};
				if (message.seq === undefined) {
					// Before ready it is the answer of init.
					if (this.failed !== null) {
						this.failed(new RenderError(failure.reason, failure.detail));
						this.failed = null;
						this.ready = null;
						return;
					}
					this.hooks.onError(failure);
					return;
				}
				if (message.seq === this.inflight) {
					this.hooks.onError(failure);
					this.flush();
				}
				return;
			}
		}
	}
}
