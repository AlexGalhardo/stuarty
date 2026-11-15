import type { ConnectionState, EventStream, Unsubscribe } from "./events";
import type { ProjectEvent } from "./types";

// One event stream per project across the tabs of a browser (SDD-012 section
// 5.5, SDD-006 section 4.6). The tab that holds the Web Lock
// `vv-events-<project id>` owns the EventSource and relays what it receives on a
// BroadcastChannel of the same name; the others only listen. When the owner
// closes, the lock passes to the next tab, which opens the stream with the
// revision it holds itself.

// The part of navigator.locks the module uses.
export interface LockManagerLike {
	request(
		name: string,
		options: { signal: AbortSignal },
		callback: () => Promise<void>,
	): Promise<unknown>;
}

export interface ChannelLike {
	postMessage(message: RelayMessage): void;
	addEventListener(
		type: "message",
		listener: (event: { data: RelayMessage }) => void,
	): void;
	close(): void;
}

export type RelayMessage =
	| { kind: "event"; event: ProjectEvent }
	| { kind: "state"; state: ConnectionState }
	// A tab that has just joined asks the owner for the state of the stream.
	| { kind: "hello" };

export interface SharedStreamDeps {
	projectId: string;
	// The stream of this tab, used only while the tab is the owner.
	stream: EventStream;
	// The revision this tab holds, for the stream it opens after a hand over.
	getRevision(): number;
	locks: LockManagerLike;
	openChannel(name: string): ChannelLike;
}

export function lockName(projectId: string): string {
	return `vv-events-${projectId}`;
}

function listeners<T>(): {
	add(callback: (value: T) => void): Unsubscribe;
	emit(value: T): void;
} {
	const set = new Set<(value: T) => void>();
	return {
		add(callback) {
			set.add(callback);
			return () => set.delete(callback);
		},
		emit(value) {
			for (const callback of [...set]) callback(value);
		},
	};
}

// An EventStream that opens the real one only in the tab that owns the lock.
export function createSharedEventStream(deps: SharedStreamDeps): EventStream {
	const { stream, projectId } = deps;
	const events = listeners<ProjectEvent>();
	const states = listeners<ConnectionState>();

	let channel: ChannelLike | null = null;
	let pending: AbortController | null = null;
	let release: (() => void) | null = null;
	let owner = false;
	let lastState: ConnectionState | null = null;
	let detach: Unsubscribe[] = [];

	function becomeOwner(since: number): Promise<void> {
		owner = true;
		const channelOut = channel;
		detach = [
			stream.onEvent((event) => {
				channelOut?.postMessage({ kind: "event", event });
				events.emit(event);
			}),
			stream.onState((state) => {
				lastState = state;
				channelOut?.postMessage({ kind: "state", state });
				states.emit(state);
			}),
		];
		stream.open(since);
		return new Promise<void>((resolve) => {
			release = resolve;
		});
	}

	function stopOwning(): void {
		if (!owner) return;
		owner = false;
		stream.close();
		for (const off of detach) off();
		detach = [];
		release?.();
		release = null;
	}

	return {
		open(since) {
			if (channel !== null) return;
			channel = deps.openChannel(lockName(projectId));
			channel.addEventListener("message", (message) => {
				const relay = message.data;
				if (owner) {
					// The owner emits to its own subscribers directly; it answers a new
					// tab with the state of the stream.
					if (relay.kind === "hello" && lastState !== null) {
						channel?.postMessage({ kind: "state", state: lastState });
					}
					return;
				}
				if (relay.kind === "event") events.emit(relay.event);
				else if (relay.kind === "state") states.emit(relay.state);
			});
			channel.postMessage({ kind: "hello" });
			pending = new AbortController();
			deps.locks
				.request(lockName(projectId), { signal: pending.signal }, () => {
					// A tab that takes over opens with the revision it holds now, which is
					// at least the one it was given.
					return becomeOwner(Math.max(since, deps.getRevision()));
				})
				.catch(() => undefined);
		},
		close() {
			pending?.abort();
			pending = null;
			stopOwning();
			channel?.close();
			channel = null;
		},
		checkStale() {
			if (owner) stream.checkStale();
		},
		onEvent: events.add,
		onState: states.add,
		// Recovery and authorization failures are seen by the owner's stream; the
		// other tabs learn of them from the state they relay and the events that
		// follow.
		onRecover: (callback) => stream.onRecover(callback),
		onUnauthorized: (callback) => stream.onUnauthorized(callback),
	};
}
