import { describe, expect, test, vi } from "vitest";

import type { PlanLayer, PlanMediaLayer, PlanWindow } from "../api/types";
import { bufferSize, fitScale, sizedScale, truthWidth } from "./fit";
import { pixiColorMatrix } from "./layerFilters";
import { Lru } from "./lru";
import {
	audioClockReady,
	audioDrive,
	audioSourceSeconds,
	curveOffset,
	curveSpeedAt,
	DRIFT_FRAMES,
	MAX_VIDEO_LAYERS,
	needsCorrection,
	PlaybackClock,
	rankVideoLayers,
	shouldCommitFrame,
	videoDrive,
} from "./mediaSync";
import {
	clampToLastFrame,
	PausedFrame,
	type PausedFrameDeps,
	TRUTH_DELAY_MS,
} from "./pausedFrame";
import {
	bitmapKeyAt,
	colorMatrixAt,
	layersAt,
	mirrorSigns,
	valueAt,
} from "./planValue";
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

describe("mirror", () => {
	test("a layer is mirrored on the axes the plan names, and only those", () => {
		expect(mirrorSigns(media("a"))).toEqual({ x: 1, y: 1 });
		expect(mirrorSigns({ ...media("a"), flip_x: true })).toEqual({
			x: -1,
			y: 1,
		});
		expect(mirrorSigns({ ...media("a"), flip_y: true })).toEqual({
			x: 1,
			y: -1,
		});
		expect(mirrorSigns({ ...media("a"), flip_x: true, flip_y: true })).toEqual({
			x: -1,
			y: -1,
		});
	});
});

describe("media sync", () => {
	test("React state follows the frame only when it must", () => {
		expect(shouldCommitFrame(false, false, 1, 0, 250)).toBe(true);
		expect(shouldCommitFrame(true, false, 100, 0, 250)).toBe(false);
		expect(shouldCommitFrame(true, false, 250, 0, 250)).toBe(true);
		expect(shouldCommitFrame(true, true, 1, 0, 250)).toBe(true);
	});

	test("an audio item's target time advances with the frame", () => {
		const audio = { first: 10, source_seconds: 2, speed: 1 };
		expect(audioSourceSeconds(audio, 10, fps)).toBeCloseTo(2);
		expect(audioSourceSeconds(audio, 40, fps)).toBeCloseTo(3);
		expect(audioSourceSeconds({ ...audio, speed: 2 }, 40, fps)).toBeCloseTo(4);
		expect(needsCorrection(3.01, audioSourceSeconds(audio, 40, fps), fps)).toBe(
			false,
		);
	});

	test("the video layers that decode are the topmost four", () => {
		const layers = ["a", "b", "c", "d", "e", "f"].map((i) => media(i));
		const decoding = rankVideoLayers(layers);
		expect(MAX_VIDEO_LAYERS).toBe(4);
		expect([...decoding]).toEqual(["c", "d", "e", "f"]);
	});

	test("the playback rate is the slope of source_seconds, clamped", () => {
		expect(videoDrive([0, 1 / 30, 2 / 30], 0, fps)).toEqual({
			seek: false,
			rate: expect.closeTo(1),
		});
		expect(videoDrive([0, 2 / 30, 4 / 30], 0, fps)).toEqual({
			seek: false,
			rate: expect.closeTo(2),
		});
		expect(videoDrive([0, 20 / 30], 0, fps)).toEqual({ seek: false, rate: 4 });
		expect(videoDrive([1], 0, fps)).toEqual({ seek: false, rate: 1 });
	});

	test("a ramp is followed by the slope of the next frames", () => {
		const seconds = Array.from({ length: 40 }, (_, i) =>
			i < 20 ? i / 30 : 20 / 30 + ((i - 20) * 3) / 30,
		);
		const early = videoDrive(seconds, 0, fps);
		const late = videoDrive(seconds, 30, fps);
		expect(early.seek).toBe(false);
		expect(early.seek === false && early.rate).toBeLessThan(1.8);
		expect(late).toEqual({ seek: false, rate: expect.closeTo(3) });
	});

	test("a source that runs backwards is followed by seeks", () => {
		expect(videoDrive([3, 2.9667, 2.9333], 0, fps)).toEqual({ seek: true });
	});

	test("the audio of a curve follows the integral of the ramp", () => {
		const ramp = {
			points: [
				{ t: 0, speed: { num: 1, den: 1 } },
				{ t: 1, speed: { num: 3, den: 1 } },
			],
			item_frames: 4,
			first_frame: 0,
		};
		expect(curveSpeedAt(ramp.points, 0.5)).toBeCloseTo(2);
		expect([0, 1, 2, 3, 4].map((r) => curveOffset(ramp, r))).toEqual([
			0, 1.25, 3, 5.25, 8,
		]);
		const audio = { first: 10, source_seconds: 2, speed: 2, speed_curve: ramp };
		const drive = audioDrive(audio, 12, fps);
		expect(drive.seconds).toBeCloseTo(2 + 3 / 30);
		expect(drive.silent).toBe(false);
		expect(audioDrive({ ...audio, reverse: true }, 12, fps).silent).toBe(true);
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
		expect(bufferSize(1080, 1920, 1, 2)).toEqual({ width: 608, height: 1081 });
		expect(bufferSize(180, 320, 463 / 180, 1)).toEqual({
			width: 464,
			height: 825,
		});
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

	test("a new revision aborts the request in flight and caches nothing", async () => {
		const signals: AbortSignal[] = [];
		const windows = new PlanWindows(
			(_from, _to, _rev, _width, signal) =>
				new Promise<PlanWindow>((_resolve, reject) => {
					signals.push(signal);
					signal.addEventListener("abort", () => reject(new Error("aborted")));
				}),
		);
		windows.setRevision(1);
		const stale = windows.ensure(0);
		windows.setRevision(2);
		await expect(stale).rejects.toThrow("aborted");
		expect(signals[0]?.aborted).toBe(true);
		expect(windows.size).toBe(0);
	});

	test("asks for the preview width and drops the windows when it changes", async () => {
		const widths: (number | undefined)[] = [];
		const windows = new PlanWindows(async (from, _to, rev, width) => {
			widths.push(width);
			return plan([], from, rev);
		});
		windows.setRevision(1);
		await windows.ensure(0);
		windows.setWidth(463);
		expect(windows.peek(0)).toBeUndefined();
		await windows.ensure(0);
		windows.setWidth(463);
		expect(windows.peek(0)).toBeDefined();
		expect(widths).toEqual([undefined, 463]);
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

describe("plan matrices for PixiJS", () => {
	test("a 3x4 plan matrix becomes 5x4 rows of r g b a offset that keep alpha", () => {
		// The invert filter of SDD-004 section 3.6.
		const invert = [-1, 0, 0, 1, 0, -1, 0, 1, 0, 0, -1, 1];
		expect(pixiColorMatrix(invert)).toEqual([
			-1, 0, 0, 0, 1, 0, -1, 0, 0, 1, 0, 0, -1, 0, 1, 0, 0, 0, 1, 0,
		]);
	});

	test("a matrix of another size is refused", () => {
		expect(() => pixiColorMatrix([1, 0, 0, 0])).toThrow("12 numbers");
	});
});

describe("colour matrix at a frame", () => {
	const m1 = [1, 0, 0, 0.1, 0, 1, 0, 0.1, 0, 0, 1, 0.1];
	const m2 = [1, 0, 0, 0.2, 0, 1, 0, 0.2, 0, 0, 1, 0.2];

	test("the constant matrix is used when the plan has no series", () => {
		const layer = {
			...(media("a", 10, 12) as PlanMediaLayer),
			color_matrix: m1,
		};
		expect(colorMatrixAt(layer, 11)).toEqual(m1);
	});

	test("a series replaces it, one entry per frame from the first, null for neutral", () => {
		const layer = {
			...(media("a", 10, 12) as PlanMediaLayer),
			color_matrix: null,
			color_matrices: [null, m1, m2],
		};
		expect(colorMatrixAt(layer, 10)).toBeNull();
		expect(colorMatrixAt(layer, 11)).toEqual(m1);
		expect(colorMatrixAt(layer, 12)).toEqual(m2);
		expect(colorMatrixAt(layer, 99)).toEqual(m2);
	});
});

describe("clampToLastFrame", () => {
	test("the end of the timeline is not a frame", () => {
		expect(clampToLastFrame(600, 600)).toBe(599);
		expect(clampToLastFrame(10, 600)).toBe(10);
		expect(clampToLastFrame(5, 0)).toBe(0);
	});
});

describe("audioClockReady", () => {
	test("a running context is the clock without a resume", async () => {
		const resume = vi.fn(() => Promise.resolve());
		expect(await audioClockReady({ state: "running", resume })).toBe(true);
		expect(resume).not.toHaveBeenCalled();
	});

	test("no context means the wall clock", async () => {
		expect(await audioClockReady(null)).toBe(false);
	});

	test("a context that runs after resume is the clock", async () => {
		const context = {
			state: "suspended" as AudioContextState,
			resume: () => {
				context.state = "running";
				return Promise.resolve();
			},
		};
		expect(await audioClockReady(context)).toBe(true);
	});

	test("a resume that never settles falls back to the wall clock", async () => {
		const context = {
			state: "suspended" as AudioContextState,
			resume: () => new Promise<void>(() => undefined),
		};
		expect(await audioClockReady(context, 10)).toBe(false);
	});
});
