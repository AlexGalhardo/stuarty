import {
	type ConnectionState,
	REOPEN_DELAYS_MS,
	STALE_AFTER_MS,
	type Timers,
	type Unsubscribe,
} from "../../editor/api/events";
import type { StreamConnection, Transport } from "../../editor/api/transport";
import type { ImageStreamEvent } from "./types";

// The event stream of one image document (SDD-006 section 12, SDD-012 section 24.3): the same reconnection
// rules as the stream of a project (section 4.6), over `GET /images/{id}/events`. It carries `image.updated`,
// which names the revision another actor (or this window) committed, and `resync.required`.
export interface ImageEventStream {
	// Opens (or reopens) the stream with `since` as the last revision the caller holds.
	open(since: number): void;
	close(): void;
	// Reopens when no event arrived for STALE_AFTER_MS; called when the window becomes visible and on online.
	checkStale(): void;
	onEvent(callback: (event: ImageStreamEvent) => void): Unsubscribe;
	onState(callback: (state: ConnectionState) => void): Unsubscribe;
	// Called after the browser closed the stream (an HTTP error) and the probe of the image succeeded: the
	// document may be stale, so the caller loads it again and opens a new stream.
	onRecover(callback: () => void): Unsubscribe;
	// Called when the probe answers 401.
	onUnauthorized(callback: () => void): Unsubscribe;
}

export interface ImageEventStreamDeps {
	transport: Transport;
	imageId: string;
	// The revision of the local copy, for reopening after a stale connection.
	getRevision(): number;
	// The HTTP status of GET /images/{id}, or 0 when the network failed.
	probe(): Promise<number>;
	timers: Timers;
}

function subscribers<T extends unknown[]>(): {
	add(callback: (...args: T) => void): Unsubscribe;
	emit(...args: T): void;
} {
	const set = new Set<(...args: T) => void>();
	return {
		add(callback) {
			set.add(callback);
			return () => set.delete(callback);
		},
		emit(...args) {
			for (const callback of [...set]) callback(...args);
		},
	};
}

function parseEvent(name: string, data: string): ImageStreamEvent | null {
	if (name !== "image.updated" && name !== "resync.required") return null;
	try {
		const payload: unknown = JSON.parse(data);
		if (typeof payload !== "object" || payload === null) return null;
		return { ...(payload as object), name } as ImageStreamEvent;
	} catch {
		return null;
	}
}

export function createImageEventStream(
	deps: ImageEventStreamDeps,
): ImageEventStream {
	const { timers } = deps;
	const events = subscribers<[ImageStreamEvent]>();
	const states = subscribers<[ConnectionState]>();
	const recovers = subscribers<[]>();
	const unauthorized = subscribers<[]>();

	let connection: StreamConnection | null = null;
	let state: ConnectionState = "connecting";
	let lastEventAt = timers.now();
	let watchdog: unknown;
	let reopenTimer: unknown;
	let attempts = 0;
	let disposed = false;

	function setState(next: ConnectionState): void {
		if (next === state) return;
		state = next;
		states.emit(next);
	}

	function touch(): void {
		lastEventAt = timers.now();
		timers.clearTimeout(watchdog);
		watchdog = timers.setTimeout(restart, STALE_AFTER_MS);
	}

	function drop(): void {
		connection?.close();
		connection = null;
		timers.clearTimeout(watchdog);
		timers.clearTimeout(reopenTimer);
	}

	function restart(): void {
		if (disposed) return;
		setState("reconnecting");
		open(deps.getRevision());
	}

	function scheduleReopen(): void {
		const delay =
			REOPEN_DELAYS_MS[Math.min(attempts, REOPEN_DELAYS_MS.length - 1)];
		attempts++;
		reopenTimer = timers.setTimeout(async () => {
			if (disposed) return;
			const status = await deps.probe();
			if (disposed) return;
			if (status === 401) {
				unauthorized.emit();
				return;
			}
			if (status >= 200 && status < 300) {
				// attempts is not reset here: only an opened stream resets it, so a server that refuses the
				// stream (E_QUEUE_FULL) is retried with the back-off.
				recovers.emit();
				return;
			}
			scheduleReopen();
		}, delay);
	}

	function open(since: number): void {
		drop();
		disposed = false;
		touch();
		connection = deps.transport.openStream(
			`/images/${encodeURIComponent(deps.imageId)}/events?since=${since}`,
			{
				onOpen() {
					attempts = 0;
					touch();
					setState("live");
				},
				onMessage(name, data) {
					touch();
					if (name === null) return;
					const event = parseEvent(name, data);
					if (event) events.emit(event);
				},
				onError(closed) {
					if (!closed) {
						setState("reconnecting");
						return;
					}
					drop();
					setState("offline");
					scheduleReopen();
				},
			},
		);
	}

	return {
		open(since) {
			setState(state === "live" ? "reconnecting" : "connecting");
			open(since);
		},
		close() {
			disposed = true;
			drop();
			setState("offline");
		},
		checkStale() {
			if (disposed || connection === null) return;
			if (timers.now() - lastEventAt >= STALE_AFTER_MS) restart();
		},
		onEvent: events.add,
		onState: states.add,
		onRecover: recovers.add,
		onUnauthorized: unauthorized.add,
	};
}
