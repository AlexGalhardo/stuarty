import {
	byteSpan,
	type ChunkIndex,
	indexEnd,
	keyframeBefore,
} from "./chunkIndex";
import type { Closable } from "./frameCache";

export interface FrameLike extends Closable {
	readonly timestamp: number;
}

export interface DecoderConfig {
	codec: string;
	codedWidth: number;
	codedHeight: number;
}

export interface DecoderLike {
	readonly decodeQueueSize: number;
	configure(config: DecoderConfig): void;
	decode(chunk: unknown): void;
	flush(): Promise<void>;
	reset(): void;
	close(): void;
}

// What the browser gives, injected so the decode logic runs under test with
// fakes (SDD-013 section 5.3).
export interface DecodeDeps<F extends FrameLike> {
	createDecoder(handlers: {
		output(frame: F): void;
		error(error: Error): void;
	}): DecoderLike;
	makeChunk(init: {
		type: "key" | "delta";
		timestamp: number;
		data: Uint8Array;
	}): unknown;
	// Bytes `start` to `end` inclusive of the proxy; `total` is the size of the
	// proxy the server reported, when it did.
	fetchRange(
		start: number,
		end: number,
	): Promise<{ bytes: Uint8Array; total: number | null }>;
}

export interface DecodeLimits {
	// Decoded frames held ahead of the one shown, per layer.
	ahead: number;
	// `decodeQueueSize` at which no more chunks are queued.
	queue: number;
	// Samples between the last decoded one and the target for which decoding
	// goes on forward instead of restarting from a key frame.
	forward: number;
	// How long to wait for a frame before flushing the decoder.
	stallMs: number;
}

export const DEFAULT_LIMITS: DecodeLimits = {
	ahead: 4,
	queue: 8,
	forward: 4,
	stallMs: 250,
};

export class DecodeFailure extends Error {}

export type DecodeMode = "seek" | "play";

interface Waiter {
	sample: number;
	wake(): void;
}

// The decoder of one asset: the seek and play rules of SDD-013 section 5.3
// items 3 and 4. It owns the frames it holds and gives one away with
// `frameAt`; whoever receives it closes it.
export class AssetDecoder<F extends FrameLike> {
	private readonly decoder: DecoderLike;
	private readonly keys: ReadonlySet<number>;
	private readonly held = new Map<number, F>();
	private readonly waiters = new Set<Waiter>();
	// The last sample queued in the decoder, and the number of the next output.
	private fed = -1;
	private outputNext = 0;
	// The last sample given away: a request for a later one may go on forward.
	private delivered = -1;
	// Samples below this are closed on arrival or already closed.
	private floor = 0;
	// After a flush the decoder wants a key frame.
	private needKey = true;
	private streaming = false;
	private pumping = false;
	private failure: Error | null = null;
	private closed = false;
	private chain: Promise<unknown> = Promise.resolve();
	private feeds: Promise<unknown> = Promise.resolve();
	// Counts restarts: a fetch that finishes after one belongs to a run that is
	// gone and queues nothing.
	private run = 0;
	private latest = 0;
	private thrown = 0;

	private constructor(
		private readonly index: ChunkIndex,
		private readonly deps: DecodeDeps<F>,
		private readonly limits: DecodeLimits,
	) {
		this.keys = new Set(index.keyframes);
		this.decoder = deps.createDecoder({
			output: (frame) => this.onOutput(frame),
			error: (error) => this.fail(error),
		});
	}

	// A config the browser cannot decode is not an AssetDecoder: the caller
	// falls back to `<video>`.
	static async open<F extends FrameLike>(
		index: ChunkIndex,
		deps: DecodeDeps<F>,
		supported: (config: DecoderConfig) => Promise<boolean>,
		limits: DecodeLimits = DEFAULT_LIMITS,
	): Promise<AssetDecoder<F> | null> {
		const config = {
			codec: index.codec,
			codedWidth: index.coded_width,
			codedHeight: index.coded_height,
		};
		if (!(await supported(config))) return null;
		return new AssetDecoder(index, deps, limits);
	}

	// Frames waiting to be asked for; the one given away is the caller's.
	get openFrames(): number {
		return this.held.size;
	}

	// Samples decoded and thrown away since the start, for the budget of
	// NFR-003 (at most one GOP less one per seek).
	get discarded(): number {
		return this.thrown;
	}

	private config(): DecoderConfig {
		return {
			codec: this.index.codec,
			codedWidth: this.index.coded_width,
			codedHeight: this.index.coded_height,
		};
	}

	private discard(frame: F): void {
		this.thrown++;
		frame.close();
	}

	private onOutput(frame: F): void {
		const sample = this.outputNext++;
		if (this.closed || sample < this.floor || sample <= this.delivered) {
			this.discard(frame);
		} else {
			this.held.set(sample, frame);
		}
		this.wake();
		this.pump();
	}

	private fail(error: Error): void {
		this.failure = error;
		this.wake();
	}

	private wake(): void {
		for (const waiter of [...this.waiters]) {
			if (this.failure !== null || this.held.has(waiter.sample)) waiter.wake();
		}
	}

	// Resolves with the frame of `sample`, or null when a newer request came in
	// before this one started (a scrub moves faster than the decoder).
	frameAt(sample: number, mode: DecodeMode): Promise<F | null> {
		const ticket = ++this.latest;
		const run = this.chain.then(() => {
			if (ticket !== this.latest || this.closed) return null;
			return this.serve(sample, mode);
		});
		this.chain = run.catch(() => undefined);
		return run;
	}

	// Whether decoding goes on from the last queued sample: the target is after
	// it by at most `forward` samples, no key frame lies between, and nothing
	// the target needs was closed already.
	private canContinue(sample: number): boolean {
		if (this.needKey || this.fed < 0) return false;
		if (sample < this.floor || sample <= this.delivered) return false;
		if (sample > this.fed + this.limits.forward) return false;
		return keyframeBefore(this.index, sample) <= this.fed;
	}

	private async serve(sample: number, mode: DecodeMode): Promise<F | null> {
		if (this.failure !== null) throw this.failure;
		this.streaming = mode === "play";
		if (!this.held.has(sample) && !this.canContinue(sample)) {
			this.restart(sample);
		}
		this.floor = Math.max(this.floor, sample);
		for (const [s, frame] of [...this.held]) {
			if (s >= sample) continue;
			this.held.delete(s);
			this.discard(frame);
		}
		if (!this.held.has(sample)) {
			await this.feed(sample);
			await this.until(sample);
		}
		if (this.closed) return null;
		const frame = this.held.get(sample);
		if (frame === undefined) throw new DecodeFailure("the frame did not come");
		this.held.delete(sample);
		this.delivered = sample;
		this.pump();
		return frame;
	}

	private restart(sample: number): void {
		const k = keyframeBefore(this.index, sample);
		this.decoder.reset();
		this.decoder.configure(this.config());
		for (const frame of this.held.values()) this.discard(frame);
		this.held.clear();
		this.fed = k - 1;
		this.outputNext = k;
		this.delivered = -1;
		this.floor = sample;
		this.needKey = false;
		this.run++;
	}

	// Queues the samples after the last queued one up to `to` with one Range
	// request. A seek queues at most one GOP; playback queues what `pump` allows.
	// Calls run one after the other, so two never queue the same sample.
	private feed(to: number): Promise<void> {
		const run = this.run;
		const next = this.feeds.then(() => this.feedNow(run, to));
		this.feeds = next.catch(() => undefined);
		return next;
	}

	private async feedNow(run: number, to: number): Promise<void> {
		if (run !== this.run || this.closed) return;
		const from = this.fed + 1;
		if (from > to) return;
		const { start, end } = byteSpan(this.index, from, to);
		const { bytes, total } = await this.deps.fetchRange(start, end - 1);
		if (run !== this.run || this.closed) return;
		if (total !== null && indexEnd(this.index) > total) {
			throw new DecodeFailure("the index reaches past the proxy");
		}
		for (let s = from; s <= to; s++) {
			const offset = (this.index.offsets[s] as number) - start;
			const size = this.index.sizes[s] as number;
			this.decoder.decode(
				this.deps.makeChunk({
					type: this.keys.has(s) ? "key" : "delta",
					timestamp: this.index.timestamps_us[s] as number,
					data: bytes.subarray(offset, offset + size),
				}),
			);
		}
		this.fed = to;
	}

	// Waits for the output of `sample`. A decoder that keeps its last frames
	// until a flush gets one after `stallMs`; it then wants a key frame again.
	private async until(sample: number): Promise<void> {
		const arrived = (): boolean =>
			this.held.has(sample) || this.failure !== null || this.closed;
		const wait = (): Promise<void> =>
			new Promise((resolve) => {
				const waiter: Waiter = {
					sample,
					wake: () => {
						clearTimeout(timer);
						this.waiters.delete(waiter);
						resolve();
					},
				};
				const timer = setTimeout(waiter.wake, this.limits.stallMs);
				this.waiters.add(waiter);
				if (arrived()) waiter.wake();
			});
		await wait();
		if (!arrived()) {
			await this.decoder.flush();
			this.needKey = true;
		}
		if (this.failure !== null) throw this.failure;
	}

	// Playback keeps decoding after the last sample, in order, with at most
	// `ahead` frames decoded and waiting and no more than `queue` chunks in the
	// decoder (SDD-013 section 5.3 item 4).
	private pump(): void {
		if (!this.streaming || this.pumping || this.closed) return;
		if (this.failure !== null || this.needKey || this.fed < 0) return;
		const last = this.index.offsets.length - 1;
		if (this.fed >= last) return;
		const inflight = this.fed + 1 - this.outputNext;
		const room = this.limits.ahead - this.held.size - inflight;
		if (room <= 0 || this.decoder.decodeQueueSize >= this.limits.queue) return;
		this.pumping = true;
		this.feed(Math.min(last, this.fed + room))
			.catch((e: unknown) =>
				this.fail(e instanceof Error ? e : new DecodeFailure(String(e))),
			)
			.finally(() => {
				this.pumping = false;
				this.pump();
			});
	}

	close(): void {
		if (this.closed) return;
		this.closed = true;
		for (const frame of this.held.values()) frame.close();
		this.held.clear();
		for (const waiter of [...this.waiters]) waiter.wake();
		try {
			this.decoder.close();
		} catch {
			// Already closed by an error.
		}
	}
}
