import { describe, expect, test } from "vitest";

import type { PlanAudio } from "../api/types";
import { processedDrive } from "./mediaSync";
import {
	type Probe,
	type ProbeResult,
	ProcessedAudio,
	RETRY_AFTER_FAILURE_MS,
} from "./processedAudio";

const KEY = "a".repeat(64);

function audio(processed?: PlanAudio["processed"]): PlanAudio {
	return {
		item: "itm_a",
		asset: "ast_a",
		first: 30,
		last: 179,
		source_seconds: 0,
		speed: 1,
		gain: 1,
		processed,
	};
}

// A probe that answers from a function, and a timer queue that runs when told.
function setup(answers: (url: string) => ProbeResult) {
	const asked: string[] = [];
	const timers: Array<() => void> = [];
	let now = 0;
	let changes = 0;
	const probe: Probe = (url) => {
		asked.push(url);
		return Promise.resolve(answers(url));
	};
	const processed = new ProcessedAudio({
		projectId: () => "prj_1",
		probe,
		timers: {
			setTimeout: (fn) => {
				timers.push(fn);
				return timers.length;
			},
			clearTimeout: () => undefined,
		},
		now: () => now,
		onChange: () => {
			changes++;
		},
	});
	return {
		processed,
		asked,
		timers,
		changes: () => changes,
		advance: (ms: number) => {
			now += ms;
		},
	};
}

// Lets the promises of the probes settle: they are all already resolved.
async function flush(): Promise<void> {
	for (let i = 0; i < 10; i++) await Promise.resolve();
}

// The playhead at frame 60 of an item that starts at 30 is frame 30 of its file.
const WINDOW = `/api/v1/projects/prj_1/audio-previews/${KEY}?from=30`;
const FULL = `/api/v1/projects/prj_1/audio-previews/${KEY}`;

describe("ProcessedAudio", () => {
	test("an entry with nothing to process is left to the proxy", () => {
		const t = setup(() => ({ kind: "failed" }));
		expect(t.processed.resolve(audio(), 60)).toBeNull();
		expect(t.asked).toEqual([]);
		expect(t.processed.states()).toEqual({});
	});

	test("the window is asked for first, at the playhead, and the item says processing", () => {
		const t = setup(() => ({ kind: "processing" }));
		expect(
			t.processed.resolve(audio({ key: KEY, start: 30, frames: 150 }), 60),
		).toBeNull();
		expect(t.asked).toEqual([WINDOW]);
		expect(t.processed.states()).toEqual({ itm_a: "processing" });
	});

	test("a ready window is played inside its frames, then the whole file once it is ready", async () => {
		const t = setup((url) =>
			url === WINDOW
				? { kind: "ready", window: [30, 89] }
				: url === FULL
					? { kind: "ready", window: null }
					: { kind: "processing" },
		);
		const entry = audio({ key: KEY, start: 30, frames: 150 });
		expect(t.processed.resolve(entry, 60)).toBeNull();
		await flush();
		// The window answered; the whole file is asked for next, and the window plays now.
		expect(t.processed.resolve(entry, 61)).toEqual({ url: WINDOW, zero: 60 });
		expect(t.asked).toEqual([WINDOW, FULL]);
		expect(t.processed.states()).toEqual({ itm_a: "ready" });
		await flush();
		expect(t.processed.resolve(entry, 150)).toEqual({ url: FULL, zero: 30 });
		expect(t.changes()).toBe(2);
	});

	test("a 202 is asked again after the poll interval", async () => {
		let n = 0;
		const t = setup(() =>
			++n < 3 ? { kind: "processing" } : { kind: "ready", window: [30, 89] },
		);
		const entry = audio({ key: KEY, start: 30, frames: 150 });
		t.processed.resolve(entry, 60);
		await flush();
		expect(t.timers).toHaveLength(1);
		t.timers.shift()?.();
		await flush();
		t.timers.shift()?.();
		await flush();
		expect(t.asked.filter((u) => u === WINDOW)).toHaveLength(3);
		expect(t.processed.resolve(entry, 70)?.url).toBe(WINDOW);
	});

	test("a failed render leaves the proxy and is not asked again at once", async () => {
		const t = setup(() => ({ kind: "failed" }));
		const entry = audio({ key: KEY, start: 30, frames: 150 });
		t.processed.resolve(entry, 60);
		await flush();
		// The window failed; the whole file is asked for next, and fails too.
		expect(t.processed.resolve(entry, 60)).toBeNull();
		await flush();
		expect(t.processed.states()).toEqual({});
		const before = t.asked.length;
		t.processed.resolve(entry, 60);
		expect(t.asked).toHaveLength(before);
		t.advance(RETRY_AFTER_FAILURE_MS);
		t.processed.resolve(entry, 60);
		expect(t.asked.length).toBeGreaterThan(before);
	});

	test("a changed parameter is a new key and asks for a new file", async () => {
		const t = setup(() => ({ kind: "ready", window: [30, 89] }));
		t.processed.resolve(audio({ key: KEY, start: 30, frames: 150 }), 60);
		await flush();
		const other = "b".repeat(64);
		expect(
			t.processed.resolve(audio({ key: other, start: 30, frames: 150 }), 60),
		).toBeNull();
		expect(t.asked.some((u) => u.includes(other))).toBe(true);
	});

	test("two items with one key at different frames share a window of the file", async () => {
		const t = setup((url) =>
			url === WINDOW
				? { kind: "ready", window: [30, 89] }
				: { kind: "processing" },
		);
		const a = audio({ key: KEY, start: 30, frames: 150 });
		const b = {
			...audio({ key: KEY, start: 200, frames: 150 }),
			item: "itm_b",
		};
		t.processed.resolve(a, 60);
		await flush();
		// Frame 235 is frame 35 of B's file: inside the window that A asked for,
		// played from B's own start.
		expect(t.processed.resolve(b, 235)).toEqual({ url: WINDOW, zero: 230 });
		// Frame 200 is frame 0 of B's file, before the window: the proxy plays.
		expect(t.processed.resolve(b, 200)).toBeNull();
	});
});

describe("processedDrive", () => {
	test("plays at rate 1 from the frame the file starts at, and is never silent", () => {
		const fps = { num: 30, den: 1 };
		expect(processedDrive(60, 30, fps)).toEqual({
			seconds: 1,
			rate: 1,
			silent: false,
		});
		expect(processedDrive(10, 30, fps).seconds).toBe(0);
	});
});
