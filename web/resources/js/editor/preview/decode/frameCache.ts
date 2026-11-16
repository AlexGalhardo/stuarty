export interface Closable {
	close(): void;
}

// Counts the frames that are open (not yet `close()`d). A frame is never kept
// without being counted here (SDD-013 section 5.3 item 6), and `stats.openFrames`
// reads it.
export class FrameLedger<F extends Closable> {
	private readonly open = new Set<F>();

	get count(): number {
		return this.open.size;
	}

	adopt(frame: F): F {
		this.open.add(frame);
		return frame;
	}

	// Idempotent: a frame is closed once.
	release(frame: F): void {
		if (!this.open.delete(frame)) return;
		frame.close();
	}

	releaseAll(): void {
		for (const frame of [...this.open]) this.release(frame);
	}
}

export interface CachedFrame<F> {
	asset: string;
	sample: number;
	// The source times, in microseconds, for which this sample is the one shown.
	lo: number;
	hi: number;
	frame: F;
}

// A small cache of decoded frames keyed by source frame: asset and sample, found
// by the source time the plan asks for. At most `max` frames are held; the
// least recently used is evicted. An evicted frame is not closed at once: the
// renderer may still read it in the draw that is running, so it waits in
// `retired` until `sweep()`, which the caller runs after the render
// (the black texture of P2-QA-003).
export class FrameCache<F extends Closable> {
	private readonly entries = new Map<string, CachedFrame<F>>();
	private retired: F[] = [];

	constructor(
		private readonly max: number,
		private readonly ledger: FrameLedger<F>,
	) {}

	get size(): number {
		return this.entries.size;
	}

	get pendingClose(): number {
		return this.retired.length;
	}

	private static key(asset: string, sample: number): string {
		return `${asset}\u0000${sample}`;
	}

	// The frame of `asset` whose window holds `micros`, marked as recently used.
	find(asset: string, micros: number): CachedFrame<F> | undefined {
		for (const [key, entry] of this.entries) {
			if (entry.asset !== asset || micros < entry.lo || micros >= entry.hi) {
				continue;
			}
			this.entries.delete(key);
			this.entries.set(key, entry);
			return entry;
		}
		return undefined;
	}

	// The most recent frame of the asset, whatever its time: what a layer shows
	// while the frame it asked for is still being decoded.
	latest(asset: string): CachedFrame<F> | undefined {
		let found: CachedFrame<F> | undefined;
		for (const entry of this.entries.values()) {
			if (entry.asset === asset) found = entry;
		}
		return found;
	}

	put(entry: CachedFrame<F>): void {
		const key = FrameCache.key(entry.asset, entry.sample);
		const old = this.entries.get(key);
		if (old !== undefined && old.frame !== entry.frame) {
			this.retired.push(old.frame);
		}
		this.entries.delete(key);
		this.entries.set(key, entry);
		while (this.entries.size > this.max) {
			const oldest = this.entries.entries().next();
			if (oldest.done) break;
			this.entries.delete(oldest.value[0]);
			this.retired.push(oldest.value[1].frame);
		}
	}

	dropAsset(asset: string): void {
		for (const [key, entry] of [...this.entries]) {
			if (entry.asset !== asset) continue;
			this.entries.delete(key);
			this.retired.push(entry.frame);
		}
	}

	// Closes the frames evicted since the last sweep; run after a render.
	sweep(): void {
		const retired = this.retired;
		this.retired = [];
		for (const frame of retired) this.ledger.release(frame);
	}

	clear(): void {
		for (const entry of this.entries.values()) this.retired.push(entry.frame);
		this.entries.clear();
		this.sweep();
	}
}
