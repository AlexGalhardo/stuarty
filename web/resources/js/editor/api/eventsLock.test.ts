import { describe, expect, test } from "vitest";

import type { ConnectionState, EventStream } from "./events";
import {
	type ChannelLike,
	createSharedEventStream,
	type LockManagerLike,
	lockName,
	type RelayMessage,
} from "./eventsLock";
import type { ProjectEvent } from "./types";

// Two simulated tabs share a lock manager (first come, first served, as
// navigator.locks) and a broadcast hub.
function browser() {
	const queue: { grant: () => Promise<void>; done: () => void }[] = [];
	let held = false;

	function next(): void {
		if (held) return;
		const entry = queue.shift();
		if (!entry) return;
		held = true;
		void entry.grant().then(() => {
			held = false;
			entry.done();
			next();
		});
	}

	const locks: LockManagerLike = {
		request: (_name, { signal }, callback) =>
			new Promise((resolve, reject) => {
				signal.addEventListener("abort", () => {
					const at = queue.findIndex((e) => e.grant === grant);
					if (at >= 0) queue.splice(at, 1);
					reject(new Error("aborted"));
				});
				const grant = (): Promise<void> => callback();
				queue.push({ grant, done: () => resolve(undefined) });
				next();
			}),
	};

	const channels = new Set<{
		owner: object;
		deliver: (m: RelayMessage) => void;
	}>();
	function openChannel(owner: object): ChannelLike {
		let listener: (event: { data: RelayMessage }) => void = () => undefined;
		const mine = { owner, deliver: (m: RelayMessage) => listener({ data: m }) };
		channels.add(mine);
		return {
			postMessage(message) {
				for (const other of channels)
					if (other !== mine) other.deliver(message);
			},
			addEventListener(_type, l) {
				listener = l;
			},
			close() {
				channels.delete(mine);
			},
		};
	}

	return { locks, openChannel };
}

function fakeStream() {
	const calls: { open: number[]; close: number } = { open: [], close: 0 };
	const eventCallbacks = new Set<(e: ProjectEvent) => void>();
	const stateCallbacks = new Set<(s: ConnectionState) => void>();
	const stream: EventStream = {
		open: (since) => {
			calls.open.push(since);
		},
		close: () => {
			calls.close++;
		},
		checkStale: () => undefined,
		onEvent: (cb) => {
			eventCallbacks.add(cb);
			return () => eventCallbacks.delete(cb);
		},
		onState: (cb) => {
			stateCallbacks.add(cb);
			return () => stateCallbacks.delete(cb);
		},
		onRecover: () => () => undefined,
		onUnauthorized: () => () => undefined,
	};
	return {
		stream,
		calls,
		emit: (e: ProjectEvent) => {
			for (const cb of [...eventCallbacks]) cb(e);
		},
		state: (s: ConnectionState) => {
			for (const cb of [...stateCallbacks]) cb(s);
		},
	};
}

function revisionEvent(revision: number): ProjectEvent {
	return { name: "revision.committed", revision } as unknown as ProjectEvent;
}

const flush = async (): Promise<void> => {
	for (let i = 0; i < 5; i++) await Promise.resolve();
};

function tab(env: ReturnType<typeof browser>, held: { revision: number }) {
	const inner = fakeStream();
	const owner = {};
	const shared = createSharedEventStream({
		projectId: "prj_a",
		stream: inner.stream,
		getRevision: () => held.revision,
		locks: env.locks,
		openChannel: () => env.openChannel(owner),
	});
	const seen: number[] = [];
	shared.onEvent((e) =>
		seen.push((e as unknown as { revision: number }).revision),
	);
	return { inner, shared, seen };
}

describe("one event stream per project across tabs", () => {
	test("the lock is named after the project", () => {
		expect(lockName("prj_a")).toBe("vv-events-prj_a");
	});

	test("two tabs open one stream in total, and both see every event", async () => {
		const env = browser();
		const a = tab(env, { revision: 5 });
		const b = tab(env, { revision: 5 });
		a.shared.open(5);
		b.shared.open(5);
		await flush();

		expect(a.inner.calls.open).toEqual([5]);
		expect(b.inner.calls.open).toEqual([]);

		a.inner.emit(revisionEvent(6));
		a.inner.emit(revisionEvent(7));
		expect(a.seen).toEqual([6, 7]);
		expect(b.seen).toEqual([6, 7]);
	});

	test("the state of the owner reaches the other tab", async () => {
		const env = browser();
		const a = tab(env, { revision: 1 });
		const b = tab(env, { revision: 1 });
		const states: ConnectionState[] = [];
		b.shared.onState((s) => states.push(s));
		a.shared.open(1);
		b.shared.open(1);
		await flush();
		a.inner.state("live");
		a.inner.state("reconnecting");
		expect(states).toEqual(["live", "reconnecting"]);
	});

	test("a tab that joins later is told the state of the stream", async () => {
		const env = browser();
		const a = tab(env, { revision: 1 });
		a.shared.open(1);
		await flush();
		a.inner.state("live");
		const b = tab(env, { revision: 1 });
		const states: ConnectionState[] = [];
		b.shared.onState((s) => states.push(s));
		b.shared.open(1);
		await flush();
		expect(states).toEqual(["live"]);
	});

	test("closing the owner hands the stream over without a gap", async () => {
		const env = browser();
		const heldA = { revision: 5 };
		const heldB = { revision: 5 };
		const a = tab(env, heldA);
		const b = tab(env, heldB);
		a.shared.open(5);
		b.shared.open(5);
		await flush();

		a.inner.emit(revisionEvent(6));
		heldA.revision = 6;
		heldB.revision = 6;
		a.shared.close();
		await flush();

		// B opens with the revision it holds, so the core sends 7 next.
		expect(a.inner.calls.close).toBe(1);
		expect(b.inner.calls.open).toEqual([6]);

		b.inner.emit(revisionEvent(7));
		b.inner.emit(revisionEvent(8));
		expect(b.seen).toEqual([6, 7, 8]);
	});

	test("a tab that closes before it owns the lock leaves the queue", async () => {
		const env = browser();
		const a = tab(env, { revision: 1 });
		const b = tab(env, { revision: 1 });
		a.shared.open(1);
		b.shared.open(1);
		await flush();
		b.shared.close();
		a.shared.close();
		await flush();
		expect(b.inner.calls.open).toEqual([]);
	});
});
