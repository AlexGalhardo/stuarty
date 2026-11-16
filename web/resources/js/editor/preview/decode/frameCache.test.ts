import { describe, expect, it } from "vitest";
import { FrameCache, FrameLedger } from "./frameCache";

class Frame {
	closed = 0;
	close(): void {
		this.closed++;
	}
}

function setup(max: number) {
	const ledger = new FrameLedger<Frame>();
	const cache = new FrameCache<Frame>(max, ledger);
	const put = (asset: string, sample: number): Frame => {
		const frame = ledger.adopt(new Frame());
		cache.put({ asset, sample, lo: sample * 10, hi: sample * 10 + 10, frame });
		return frame;
	};
	return { ledger, cache, put };
}

describe("FrameLedger", () => {
	it("counts adopted frames and closes each once", () => {
		const ledger = new FrameLedger<Frame>();
		const a = ledger.adopt(new Frame());
		ledger.adopt(new Frame());
		expect(ledger.count).toBe(2);
		ledger.release(a);
		ledger.release(a);
		expect(a.closed).toBe(1);
		expect(ledger.count).toBe(1);
		ledger.releaseAll();
		expect(ledger.count).toBe(0);
	});
});

describe("FrameCache", () => {
	it("finds a frame by the source time inside its window", () => {
		const { cache, put } = setup(4);
		const f = put("a", 3);
		expect(cache.find("a", 30)?.frame).toBe(f);
		expect(cache.find("a", 39)?.frame).toBe(f);
		expect(cache.find("a", 40)).toBeUndefined();
		expect(cache.find("b", 30)).toBeUndefined();
	});

	it("never holds more than its bound and closes the evicted only on sweep", () => {
		const { cache, ledger, put } = setup(3);
		const frames = [0, 1, 2, 3, 4].map((s) => put("a", s));
		expect(cache.size).toBe(3);
		expect(cache.pendingClose).toBe(2);
		expect(frames[0]?.closed).toBe(0);
		cache.sweep();
		expect(frames[0]?.closed).toBe(1);
		expect(frames[1]?.closed).toBe(1);
		expect(frames[4]?.closed).toBe(0);
		expect(ledger.count).toBe(3);
	});

	it("evicts the least recently used", () => {
		const { cache, put } = setup(2);
		put("a", 0);
		put("a", 1);
		cache.find("a", 0);
		put("a", 2);
		cache.sweep();
		expect(cache.find("a", 0)).toBeDefined();
		expect(cache.find("a", 10)).toBeUndefined();
	});

	it("keeps the open frames at the bound over many puts", () => {
		const { cache, ledger, put } = setup(5);
		for (let i = 0; i < 500; i++) {
			put("a", i);
			cache.sweep();
			expect(ledger.count).toBeLessThanOrEqual(5);
		}
	});

	it("replaces a sample put twice and closes the first", () => {
		const { cache, put } = setup(4);
		const first = put("a", 1);
		put("a", 1);
		cache.sweep();
		expect(first.closed).toBe(1);
		expect(cache.size).toBe(1);
	});

	it("returns the latest frame of an asset and drops an asset", () => {
		const { cache, ledger, put } = setup(6);
		put("a", 0);
		const last = put("a", 1);
		put("b", 0);
		expect(cache.latest("a")?.frame).toBe(last);
		cache.dropAsset("a");
		cache.sweep();
		expect(cache.latest("a")).toBeUndefined();
		expect(ledger.count).toBe(1);
	});

	it("closes everything on clear", () => {
		const { cache, ledger, put } = setup(6);
		put("a", 0);
		put("b", 0);
		cache.clear();
		expect(ledger.count).toBe(0);
	});
});
