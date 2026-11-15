import { describe, expect, test } from "vitest";

import type { PlanLayer, PlanWindow } from "../api/types";
import { bufferSize, fitScale, sizedScale, truthWidth } from "./fit";
import { Lru } from "./lru";
import {
	DRIFT_FRAMES,
	MAX_VIDEO_LAYERS,
	needsCorrection,
	PlaybackClock,
	playbackRateFor,
	rankVideoLayers,
} from "./mediaSync";
import {
	PausedFrame,
	type PausedFrameDeps,
	TRUTH_DELAY_MS,
} from "./pausedFrame";
import { bitmapKeyAt, layersAt, valueAt } from "./planValue";
import { PlanWindows, windowStart } from "./planWindows";

const fps = { num: 30, den: 1 };

function media(item: string, first = 0, last = 59): PlanLayer {
	return {
		item,
		kind: "media",
		asset: `ast_${item}`,
		asset_kind: "video",
		first,
		last,
		source_seconds: [0, 0.0333, 0.0667],
		crop: [0, 0, 1080, 1920],
		cx: 540,
		cy: 960,
		w: 1080,
		h: 1920,
		rotation: 0,
		scale: 1,
		opacity: 1,
		color_matrix: null,
	};
}

function plan(layers: PlanLayer[], from = 0, revision = 1): PlanWindow {
	return {
		revision,
		from,
		to: from + 300,
		end_frame: 900,
		width: 1080,
		height: 1920,
		background: "#000000",
		safe_area: [0, 0, 1, 1],
		layers,
		audio: [],
	};
}

describe("plan values", () => {
	test("a constant is the same at every frame; an array is indexed from first", () => {
		expect(valueAt(0.5, 99, 10)).toBe(0.5);
		expect(valueAt([0.2, 0.4, 0.6], 11, 10)).toBe(0.4);
		expect(valueAt([0.2, 0.4, 0.6], 99, 10)).toBe(0.6);
	});

	test("only the layers active at the frame are drawn, in plan order", () => {
		const p = plan([media("a", 0, 9), media("b", 5, 20), media("c", 30, 40)]);
		expect(layersAt(p, 7).map((l) => l.item)).toEqual(["a", "b"]);
		expect(layersAt(p, 25)).toEqual([]);
	});

	test("a bitmap layer picks its key by frame", () => {
		const layer = {
			item: "t",
			kind: "bitmap" as const,
			first: 10,
			last: 12,
			bitmaps: { keys: ["k0", "k1"], index: [0, 1, 1] },
			natural: [10, 10] as [number, number],
			cx: 0,
			cy: 0,
			w: 1,
			h: 1,
			rotation: 0,
			scale: 1,
			opacity: 1,
		};
		expect(bitmapKeyAt(layer, 10)).toBe("k0");
		expect(bitmapKeyAt(layer, 12)).toBe("k1");
	});
});

describe("media sync", () => {
	test("the video layers that decode are the topmost four", () => {
		const layers = ["a", "b", "c", "d", "e", "f"].map((i) => media(i));
		const decoding = rankVideoLayers(layers);
		expect(MAX_VIDEO_LAYERS).toBe(4);
		expect([...decoding]).toEqual(["c", "d", "e", "f"]);
	});

	test("the playback rate is the slope of source_seconds, clamped", () => {
		expect(playbackRateFor([0, 1 / 30, 2 / 30], fps)).toBeCloseTo(1);
		expect(playbackRateFor([0, 2 / 30, 4 / 30], fps)).toBeCloseTo(2);
		expect(playbackRateFor([0, 20 / 30], fps)).toBe(4);
		expect(playbackRateFor([1], fps)).toBe(1);
	});

	test("currentTime is corrected only past two frames of drift", () => {
		const frame = 1 / 30;
		expect(needsCorrection(1, 1 + DRIFT_FRAMES * frame * 0.9, fps)).toBe(false);
		expect(needsCorrection(1, 1 + DRIFT_FRAMES * frame * 1.2, fps)).toBe(true);
	});

	test("the audio clock gives frame = start + floor(elapsed * fps)", () => {
		let now = 10;
		const clock = new PlaybackClock(() => now, fps);
		clock.start(100);
		now = 10.5;
		expect(clock.frame()).toBe(115);
		const ntsc = new PlaybackClock(() => now, { num: 30000, den: 1001 });
		ntsc.start(0);
		now = 11.5;
		expect(ntsc.frame()).toBe(29);
	});
});

describe("fit and size", () => {
	test("50% and 100% are shares of the buffer, capped at 1080 on the long edge", () => {
		expect(sizedScale("fit", 0.5, 1080, 1920)).toBe(0.5);
		expect(sizedScale("100", 0.5, 1080, 1920)).toBeCloseTo(1080 / 1920, 5);
		expect(sizedScale("50", 0.5, 1080, 1920)).toBeCloseTo(1080 / 1920 / 2, 5);
		expect(sizedScale("100", 0.5, 640, 360)).toBe(1);
		expect(sizedScale("50", 0.5, 640, 360)).toBe(0.5);
		expect(sizedScale("100", 0.5, 0, 0)).toBe(0);
	});

	test("the box keeps 16 px of padding and the aspect ratio", () => {
		expect(fitScale(1032, 1832, 1080, 1920)).toBeCloseTo(1000 / 1080, 5);
		expect(fitScale(0, 0, 1080, 1920)).toBe(0);
	});

	test("the pixel buffer is capped at 1080 on the long edge", () => {
		expect(bufferSize(1080, 1920, 1, 2)).toEqual({ width: 608, height: 1080 });
		expect(bufferSize(1080, 1920, 0.25, 1)).toEqual({
			width: 270,
			height: 480,
		});
	});

	test("the paused frame width is the next of the cached sizes of the core", () => {
		expect([100, 270, 271, 540, 541, 1079, 5000].map(truthWidth)).toEqual([
			270, 270, 360, 540, 720, 1080, 1080,
		]);
	});
});

describe("Lru", () => {
	test("drops the least recently used entry", () => {
		const evicted: string[] = [];
		const lru = new Lru<string, number>(2, (k) => evicted.push(k));
		lru.set("a", 1);
		lru.set("b", 2);
		lru.get("a");
		lru.set("c", 3);
		expect(evicted).toEqual(["b"]);
		expect(lru.size).toBe(2);
	});
});

describe("PlanWindows", () => {
	test("aligns windows to 300 frames and shares one request", async () => {
		const calls: [number, number, number][] = [];
		const windows = new PlanWindows(async (from, to, rev) => {
			calls.push([from, to, rev]);
			return plan([], from, rev);
		});
		windows.setRevision(3);
		expect(windowStart(299)).toBe(0);
		expect(windowStart(300)).toBe(300);
		await Promise.all([windows.ensure(10), windows.ensure(250)]);
		expect(calls).toEqual([[0, 300, 3]]);
		expect(windows.peek(1)?.from).toBe(0);
	});

	test("a new revision drops every window and ignores a late answer", async () => {
		let resolve: (p: PlanWindow) => void = () => undefined;
		const windows = new PlanWindows(
			() => new Promise<PlanWindow>((r) => (resolve = r)),
		);
		windows.setRevision(1);
		const late = windows.ensure(0);
		windows.setRevision(2);
		resolve(plan([], 0, 1));
		await late;
		expect(windows.peek(0)).toBeUndefined();
	});

	test("keeps at most eight windows and prefetches within 60 frames of the end", async () => {
		const asked: number[] = [];
		const windows = new PlanWindows(async (from) => {
			asked.push(from);
			return plan([], from);
		});
		windows.setRevision(1);
		for (let i = 0; i < 10; i++) await windows.ensure(i * 300);
		expect(windows.size).toBe(8);
		asked.length = 0;
		windows.prefetch(3000 + 100);
		expect(asked).toEqual([]);
		windows.prefetch(3000 + 250);
		await Promise.resolve();
		expect(asked).toEqual([3300]);
	});
});

describe("PausedFrame (truth on pause)", () => {
	function harness(): {
		frame: PausedFrame;
		fire(): void;
		events: string[];
		resolveLoad(): void;
	} {
		const events: string[] = [];
		let timer: (() => void) | null = null;
		let resolveLoad: () => void = () => undefined;
		const deps: PausedFrameDeps = {
			setTimeout: (fn, ms) => {
				events.push(`wait ${ms}`);
				timer = fn;
				return 1;
			},
			clearTimeout: () => {
				timer = null;
			},
			load: (url) => {
				events.push(`load ${url}`);
				return new Promise<void>((r) => {
					resolveLoad = r;
				});
			},
			show: (url) => events.push(`show ${url}`),
			hide: () => events.push("hide"),
			urlFor: (f, w, r) => `${f}/${w}/${r}`,
			mark: () => undefined,
			measure: () => undefined,
		};
		return {
			frame: new PausedFrame(deps),
			fire: () => timer?.(),
			events,
			resolveLoad: () => resolveLoad(),
		};
	}

	test("asks for the exact frame 150 ms after the stop and shows it when decoded", async () => {
		const h = harness();
		h.frame.request(12, 540, 4);
		expect(h.events).toEqual([`wait ${TRUTH_DELAY_MS}`]);
		h.fire();
		h.resolveLoad();
		await Promise.resolve();
		await Promise.resolve();
		expect(h.events).toContain("show 12/540/4");
		expect(h.frame.shown).toBe(true);
	});

	test("any change removes it, and a late decode is not shown", async () => {
		const h = harness();
		h.frame.request(12, 540, 4);
		h.fire();
		h.frame.cancel();
		h.resolveLoad();
		await Promise.resolve();
		await Promise.resolve();
		expect(h.events.some((e) => e.startsWith("show"))).toBe(false);

		h.frame.request(13, 540, 4);
		h.fire();
		h.resolveLoad();
		await Promise.resolve();
		await Promise.resolve();
		h.frame.cancel();
		expect(h.events.at(-1)).toBe("hide");
		expect(h.frame.shown).toBe(false);
	});

	test("a failed request keeps the canvas", async () => {
		const failures: unknown[] = [];
		const frame = new PausedFrame({
			setTimeout: (fn) => {
				fn();
				return 1;
			},
			clearTimeout: () => undefined,
			load: () => Promise.reject(new Error("404")),
			show: () => failures.push("shown"),
			hide: () => undefined,
			urlFor: () => "u",
			mark: () => undefined,
			measure: () => undefined,
			onFailure: (e) => failures.push(e),
		});
		frame.request(1, 540, 1);
		await Promise.resolve();
		await Promise.resolve();
		expect(failures).toHaveLength(1);
		expect(frame.shown).toBe(false);
	});
});
