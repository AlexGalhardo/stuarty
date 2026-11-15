import type { StreamConnection, Transport } from "./transport";
import type { ProjectEvent } from "./types";

// SDD-012 section 5.5 and SDD-006 sections 3.1 and 4.6.
export type ConnectionState =
	| "connecting"
	| "live"
	| "reconnecting"
	| "offline";

export const STALE_AFTER_MS = 40_000;
export const REOPEN_DELAYS_MS = [1000, 2000, 4000, 8000, 15_000] as const;

export interface Timers {
	setTimeout(callback: () => void, ms: number): unknown;
	clearTimeout(handle: unknown): void;
	now(): number;
}

export const browserTimers: Timers = {
	setTimeout: (callback, ms) => setTimeout(callback, ms),
	clearTimeout: (handle) =>
		clearTimeout(handle as ReturnType<typeof setTimeout>),
	now: () => performance.now(),
};

export type Unsubscribe = () => void;

export interface EventStream {
	// Opens (or reopens) the stream with `since` as the last revision the
	// caller holds.
	open(since: number): void;
	close(): void;
	// Closes and reopens when no event arrived for STALE_AFTER_MS; called on
	// visibilitychange to visible and on online.
	checkStale(): void;
	onEvent(callback: (event: ProjectEvent) => void): Unsubscribe;
	onState(callback: (state: ConnectionState) => void): Unsubscribe;
	// Called after the stream was closed by the browser (an HTTP error) and the
	// probe of the project succeeded: the document may be stale, so the caller
	// runs its boot algorithm again.
	onRecover(callback: () => void): Unsubscribe;
	// Called when the probe answers 401.
	onUnauthorized(callback: () => void): Unsubscribe;
}

export interface EventStreamDeps {
	transport: Transport;
	projectId: string;
	// The revision of the local copy, for reopening after a stale connection.
	getRevision(): number;
	// The HTTP status of GET /projects/{id}, or 0 when the network failed.
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

function parseEvent(name: string, data: string): ProjectEvent | null {
	try {
		const payload: unknown = JSON.parse(data);
		if (typeof payload !== "object" || payload === null) return null;
		return { ...(payload as object), name } as ProjectEvent;
	} catch {
		return null;
	}
}

export function createEventStream(deps: EventStreamDeps): EventStream {
	const { timers } = deps;
	const events = subscribers<[ProjectEvent]>();
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
				// attempts is not reset here: only an opened stream resets it, so a
				// server that refuses the stream (E_QUEUE_FULL, SDD-006 section 4.6,
				// which an EventSource reports as a plain error) is retried with the
				// back-off instead of once a second.
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
			`/projects/${encodeURIComponent(deps.projectId)}/events?since=${since}`,
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
