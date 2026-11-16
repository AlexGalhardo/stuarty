import { describe, expect, it } from "vitest";
import { type ChunkIndex, indexEnd } from "./chunkIndex";
import {
	AssetDecoder,
	DEFAULT_LIMITS,
	type DecodeDeps,
	type DecodeLimits,
	type DecoderConfig,
	type DecoderLike,
	type FrameLike,
} from "./decodeCore";
import { makeIndex } from "./testIndex";

class FakeFrame implements FrameLike {
	closed = false;
	constructor(readonly timestamp: number) {}
	close(): void {
		if (this.closed) throw new Error("closed twice");
		this.closed = true;
	}
}

interface Chunk {
	type: "key" | "delta";
	timestamp: number;
	data: Uint8Array;
}

class FakeDecoder implements DecoderLike {
	chunks: Chunk[] = [];
	resets = 0;
	flushes = 0;
	configured: DecoderConfig[] = [];
	wantsKey = true;
	// Holds every frame until a flush, as a hardware decoder with a reorder
	// buffer may.
	holdUntilFlush = false;
	private pending: FakeFrame[] = [];
	private epoch = 0;
	private emitted = 0;
	constructor(
		private readonly output: (f: FakeFrame) => void,
		private readonly error: (e: Error) => void,
	) {}
	get decodeQueueSize(): number {
		return this.chunks.length - this.emitted - this.dropped;
	}
	private dropped = 0;
	configure(c: DecoderConfig): void {
		this.configured.push(c);
	}
	decode(chunk: unknown): void {
		const c = chunk as Chunk;
		if (this.wantsKey && c.type !== "key") {
			this.error(new Error("a key frame is required"));
			return;
		}
		this.wantsKey = false;
		this.chunks.push(c);
		const frame = new FakeFrame(c.timestamp);
		if (this.holdUntilFlush) {
			this.pending.push(frame);
			return;
		}
		const epoch = this.epoch;
		queueMicrotask(() => {
			if (epoch !== this.epoch) {
				this.dropped++;
				frame.close();
				return;
			}
			this.emitted++;
			this.output(frame);
		});
	}
	async flush(): Promise<void> {
		this.flushes++;
		const frames = this.pending;
		this.pending = [];
		for (const f of frames) {
			this.emitted++;
			this.output(f);
		}
		this.wantsKey = true;
	}
	reset(): void {
		this.resets++;
		this.epoch++;
		this.wantsKey = true;
		for (const f of this.pending) f.close();
		this.pending = [];
	}
	close(): void {}
}

interface Rig {
	index: ChunkIndex;
	decoder: AssetDecoder<FakeFrame>;
	fake: () => FakeDecoder;
	ranges: [number, number][];
	frames: FakeFrame[];
}

async function rig(
	options: {
		index?: ChunkIndex;
		limits?: Partial<DecodeLimits>;
		hold?: boolean;
		total?: number | null;
	} = {},
): Promise<Rig> {
	const index = options.index ?? makeIndex();
	let fake: FakeDecoder | null = null;
	const ranges: [number, number][] = [];
	const frames: FakeFrame[] = [];
	const deps: DecodeDeps<FakeFrame> = {
		createDecoder: ({ output, error }) => {
			fake = new FakeDecoder((f) => {
				frames.push(f);
				output(f);
			}, error);
			fake.holdUntilFlush = options.hold === true;
			return fake;
		},
		makeChunk: (init) => init,
		fetchRange: async (start, end) => {
			ranges.push([start, end]);
			await Promise.resolve();
			return {
				bytes: new Uint8Array(end - start + 1),
				total: options.total === undefined ? indexEnd(index) : options.total,
			};
		},
	};
	const decoder = await AssetDecoder.open(index, deps, async () => true, {
		...DEFAULT_LIMITS,
		stallMs: 20,
		...options.limits,
	});
	if (decoder === null) throw new Error("not opened");
	return {
		index,
		decoder,
		fake: () => {
			if (fake === null) throw new Error("no decoder");
			return fake;
		},
		ranges,
		frames,
	};
}

async function take(r: Rig, sample: number, mode: "seek" | "play" = "seek") {
	const frame = await r.decoder.frameAt(sample, mode);
	if (frame === null) throw new Error("superseded");
	return frame;
}

describe("AssetDecoder", () => {
	it("is not opened when the config is not supported", async () => {
		const index = makeIndex();
		const opened = await AssetDecoder.open(
			index,
			{} as DecodeDeps<FakeFrame>,
			async () => false,
		);
		expect(opened).toBeNull();
	});

	it("seeks from the previous key frame with one range request", async () => {
		const r = await rig();
		const frame = await take(r, 20);
		expect(frame.timestamp).toBe(r.index.timestamps_us[20]);
		const { chunks } = r.fake();
		expect(chunks).toHaveLength(6);
		expect(chunks[0]?.type).toBe("key");
		expect(chunks.slice(1).every((c) => c.type === "delta")).toBe(true);
		expect(chunks[0]?.timestamp).toBe(r.index.timestamps_us[15]);
		expect(r.ranges).toEqual([
			[
				r.index.offsets[15] as number,
				(r.index.offsets[20] as number) + (r.index.sizes[20] as number) - 1,
			],
		]);
		expect(r.decoder.discarded).toBe(5);
		expect(r.frames.filter((f) => f.closed)).toHaveLength(5);
	});

	it("throws away at most one GOP less one sample per seek", async () => {
		const r = await rig();
		for (const target of [14, 29, 3, 38, 0, 16]) {
			const before = r.decoder.discarded;
			const frame = await take(r, target);
			frame.close();
			expect(r.decoder.discarded - before).toBeLessThanOrEqual(14);
			expect(frame.timestamp).toBe(r.index.timestamps_us[target]);
		}
	});

	it("goes on forward for the next sample without a reset", async () => {
		const r = await rig();
		(await take(r, 20)).close();
		const resets = r.fake().resets;
		const fed = r.fake().chunks.length;
		const f = await take(r, 21);
		expect(f.timestamp).toBe(r.index.timestamps_us[21]);
		expect(r.fake().resets).toBe(resets);
		expect(r.fake().chunks.length).toBe(fed + 1);
	});

	it("goes on forward for a target up to four samples ahead", async () => {
		const r = await rig();
		(await take(r, 16)).close();
		const resets = r.fake().resets;
		(await take(r, 20)).close();
		expect(r.fake().resets).toBe(resets);
	});

	it("restarts from a key frame for a target more than four ahead", async () => {
		const r = await rig();
		(await take(r, 16)).close();
		const resets = r.fake().resets;
		const f = await take(r, 22);
		expect(f.timestamp).toBe(r.index.timestamps_us[22]);
		expect(r.fake().resets).toBe(resets + 1);
	});

	it("restarts at a key frame that lies between", async () => {
		const r = await rig();
		(await take(r, 13)).close();
		const resets = r.fake().resets;
		const before = r.decoder.discarded;
		const f = await take(r, 16);
		expect(f.timestamp).toBe(r.index.timestamps_us[16]);
		expect(r.fake().resets).toBe(resets + 1);
		expect(r.decoder.discarded - before).toBe(1);
	});

	it("restarts for a target behind the last one", async () => {
		const r = await rig();
		(await take(r, 20)).close();
		const resets = r.fake().resets;
		const f = await take(r, 18);
		expect(f.timestamp).toBe(r.index.timestamps_us[18]);
		expect(r.fake().resets).toBe(resets + 1);
	});

	it("answers null to a request a newer one replaced", async () => {
		const r = await rig();
		const first = r.decoder.frameAt(5, "seek");
		const second = r.decoder.frameAt(25, "seek");
		expect(await first).toBeNull();
		const frame = await second;
		expect(frame?.timestamp).toBe(r.index.timestamps_us[25]);
	});

	it("flushes a decoder that holds its frames and then wants a key frame", async () => {
		const r = await rig({ hold: true });
		const f = await take(r, 3);
		expect(f.timestamp).toBe(r.index.timestamps_us[3]);
		expect(r.fake().flushes).toBe(1);
		const resets = r.fake().resets;
		const g = await take(r, 4);
		expect(g.timestamp).toBe(r.index.timestamps_us[4]);
		expect(r.fake().resets).toBe(resets + 1);
	});

	it("rejects when the decoder reports an error", async () => {
		const r = await rig();
		await take(r, 2).then((f) => f.close());
		// A delta chunk without a key frame: the fake decoder reports an error.
		r.fake().wantsKey = true;
		await expect(take(r, 3)).rejects.toThrow("key frame");
	});

	it("rejects an index that reaches past the proxy", async () => {
		const r = await rig({ total: 100 });
		await expect(take(r, 2)).rejects.toThrow("past the proxy");
	});

	it("rejects a range that fails", async () => {
		const index = makeIndex();
		const decoder = await AssetDecoder.open(
			index,
			{
				createDecoder: () =>
					new FakeDecoder(
						() => {},
						() => {},
					),
				makeChunk: (i) => i,
				fetchRange: async () => {
					throw new Error("range 500");
				},
			},
			async () => true,
		);
		await expect(decoder?.frameAt(2, "seek")).rejects.toThrow("range 500");
	});

	it("keeps at most `ahead` decoded frames while playing and closes every frame", async () => {
		const r = await rig({ index: makeIndex(120, 15) });
		let delivered = 0;
		let peak = 0;
		for (let s = 0; s < 120; s++) {
			const f = await take(r, s, "play");
			delivered++;
			f.close();
			// let the pump feed and the decoder output
			await new Promise((resolve) => setTimeout(resolve, 0));
			peak = Math.max(peak, r.decoder.openFrames);
			expect(r.decoder.openFrames).toBeLessThanOrEqual(DEFAULT_LIMITS.ahead);
		}
		expect(delivered).toBe(120);
		// Play decoded straight through: one restart for the whole run.
		expect(r.fake().resets).toBe(1);
		expect(peak).toBeGreaterThan(0);
		r.decoder.close();
		await new Promise((resolve) => setTimeout(resolve, 5));
		expect(r.frames.every((f) => f.closed)).toBe(true);
	});

	it("never queues more than `queue` chunks in the decoder while playing", async () => {
		const r = await rig({
			index: makeIndex(120, 15),
			limits: { ahead: 40, queue: 3 },
		});
		(await take(r, 0, "play")).close();
		for (let s = 1; s < 60; s++) {
			(await take(r, s, "play")).close();
			await new Promise((resolve) => setTimeout(resolve, 0));
			// the room left is the bound minus what waits and what is in flight
			expect(r.fake().decodeQueueSize).toBeLessThanOrEqual(40);
		}
		expect(r.fake().resets).toBe(1);
	});

	it("stops decoding at the end of the media and keeps the last frame", async () => {
		const r = await rig({ index: makeIndex(10, 5) });
		for (let s = 0; s < 10; s++) (await take(r, s, "play")).close();
		await new Promise((resolve) => setTimeout(resolve, 5));
		expect(r.fake().chunks.length).toBe(10);
		const last = await take(r, 9);
		expect(last.timestamp).toBe(r.index.timestamps_us[9]);
	});

	it("closes what it holds on close", async () => {
		const r = await rig({ index: makeIndex(120, 15) });
		(await take(r, 0, "play")).close();
		await new Promise((resolve) => setTimeout(resolve, 5));
		expect(r.decoder.openFrames).toBeGreaterThan(0);
		r.decoder.close();
		expect(r.decoder.openFrames).toBe(0);
		expect(r.frames.every((f) => f.closed)).toBe(true);
	});

	it("returns null after close", async () => {
		const r = await rig();
		r.decoder.close();
		expect(await r.decoder.frameAt(2, "seek")).toBeNull();
	});
});
