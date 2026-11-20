import { describe, expect, test } from "vitest";

import { STALE_AFTER_MS, type Timers } from "../../editor/api/events";
import type {
	StreamConnection,
	StreamHandlers,
	Transport,
} from "../../editor/api/transport";
import { createImageEventStream } from "./imageEvents";
import type { ImageStreamEvent } from "./types";

// A clock the test moves and a transport that records the streams it opens.
function fakeTimers(): Timers & { advance(ms: number): void } {
	let now = 0;
	const pending = new Map<number, { at: number; callback: () => void }>();
	let next = 1;
	return {
		now: () => now,
		setTimeout: (callback, ms) => {
			const id = next++;
			pending.set(id, { at: now + ms, callback });
			return id;
		},
		clearTimeout(handle) {
			pending.delete(handle as number);
		},
		advance(ms) {
			now += ms;
			for (const [id, timer] of [...pending]) {
				if (timer.at <= now) {
					pending.delete(id);
					timer.callback();
				}
			}
		},
	};
}

function fakeTransport(): {
	transport: Transport;
	opened: { path: string; handlers: StreamHandlers; closed: boolean }[];
} {
	const opened: { path: string; handlers: StreamHandlers; closed: boolean }[] =
		[];
	const transport: Transport = {
		send: async () => ({ status: 200, body: null }),
		openStream(path, handlers): StreamConnection {
			const entry = { path, handlers, closed: false };
			opened.push(entry);
			return { close: () => (entry.closed = true) };
		},
	};
	return { transport, opened };
}

const updated =
	'{"image_id":"img_aaaaaaaaaa","revision":4,"base_revision":3,"kind":"edit","actor":"agent:x","at":"2026-11-20T12:00:00.000Z","diff":[]}';

describe("the image event stream", () => {
	test("opens the stream of the image with the revision it holds and gives the events their names", () => {
		const { transport, opened } = fakeTransport();
		const stream = createImageEventStream({
			transport,
			imageId: "img_aaaaaaaaaa",
			getRevision: () => 3,
			probe: async () => 200,
			timers: fakeTimers(),
		});
		const got: ImageStreamEvent[] = [];
		const states: string[] = [];
		stream.onEvent((event) => got.push(event));
		stream.onState((state) => states.push(state));

		stream.open(3);
		expect(opened[0].path).toBe("/images/img_aaaaaaaaaa/events?since=3");

		opened[0].handlers.onOpen();
		opened[0].handlers.onMessage(null, ""); // the preamble and the heartbeat carry no event
		opened[0].handlers.onMessage("image.updated", updated);
		opened[0].handlers.onMessage(
			"resync.required",
			'{"image_id":"img_aaaaaaaaaa","revision":9,"reason":"gap_too_large"}',
		);
		opened[0].handlers.onMessage("job.updated", "{}"); // not an event of this stream
		opened[0].handlers.onMessage("image.updated", "not json");

		expect(states).toEqual(["live"]);
		expect(got.map((event) => event.name)).toEqual([
			"image.updated",
			"resync.required",
		]);
		expect(got[0]).toMatchObject({ revision: 4, actor: "agent:x" });
	});

	test("reopens with the revision it holds after 40 seconds of silence", () => {
		const { transport, opened } = fakeTransport();
		const timers = fakeTimers();
		let held = 3;
		const stream = createImageEventStream({
			transport,
			imageId: "img_aaaaaaaaaa",
			getRevision: () => held,
			probe: async () => 200,
			timers,
		});
		stream.open(3);
		opened[0].handlers.onOpen();
		held = 6;
		timers.advance(STALE_AFTER_MS + 1);
		expect(opened).toHaveLength(2);
		expect(opened[0].closed).toBe(true);
		expect(opened[1].path).toBe("/images/img_aaaaaaaaaa/events?since=6");
	});

	test("an HTTP error closes it, and a probe that answers 200 asks the caller to load the image again", async () => {
		const { transport, opened } = fakeTransport();
		const timers = fakeTimers();
		const stream = createImageEventStream({
			transport,
			imageId: "img_aaaaaaaaaa",
			getRevision: () => 1,
			probe: async () => 200,
			timers,
		});
		let recovered = 0;
		stream.onRecover(() => recovered++);
		stream.open(1);
		opened[0].handlers.onError(true);
		timers.advance(1000);
		await Promise.resolve();
		await Promise.resolve();
		expect(recovered).toBe(1);
	});

	test("a 401 of the probe is told to the caller", async () => {
		const { transport, opened } = fakeTransport();
		const timers = fakeTimers();
		const stream = createImageEventStream({
			transport,
			imageId: "img_aaaaaaaaaa",
			getRevision: () => 1,
			probe: async () => 401,
			timers,
		});
		let signedOut = 0;
		stream.onUnauthorized(() => signedOut++);
		stream.open(1);
		opened[0].handlers.onError(true);
		timers.advance(1000);
		await Promise.resolve();
		await Promise.resolve();
		expect(signedOut).toBe(1);
	});

	test("close ends the connection and the watchdog", () => {
		const { transport, opened } = fakeTransport();
		const timers = fakeTimers();
		const stream = createImageEventStream({
			transport,
			imageId: "img_aaaaaaaaaa",
			getRevision: () => 1,
			probe: async () => 200,
			timers,
		});
		stream.open(1);
		stream.close();
		timers.advance(STALE_AFTER_MS * 2);
		expect(opened).toHaveLength(1);
		expect(opened[0].closed).toBe(true);
	});
});
