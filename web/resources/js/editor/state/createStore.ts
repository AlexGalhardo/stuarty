import { useSyncExternalStore } from "react";

// A minimal external store (the shape of zustand's vanilla store). It is
// written here because zustand is not an approved dependency yet (SDD-012
// section 3.1); the interface is the same one a later swap would keep:
// getState, subscribe, and a selector hook.
export interface ReadStore<S> {
	getState(): S;
	subscribe(listener: () => void): () => void;
}

export interface Store<S> extends ReadStore<S> {
	setState(update: (state: S) => S): void;
}

export function createStore<S>(initial: S): Store<S> {
	let state = initial;
	const listeners = new Set<() => void>();
	return {
		getState: () => state,
		subscribe(listener) {
			listeners.add(listener);
			return () => listeners.delete(listener);
		},
		setState(update) {
			const next = update(state);
			if (Object.is(next, state)) return;
			state = next;
			for (const listener of [...listeners]) listener();
		},
	};
}

// useStore subscribes a component to a slice. The selector must return a
// referentially stable value for an unchanged state (a field, not a new
// object), as useSyncExternalStore requires.
export function useStore<S, T>(
	store: ReadStore<S>,
	select: (state: S) => T,
): T {
	const snapshot = () => select(store.getState());
	// The server snapshot is the same state: the editor has no server render, but
	// the tests render to a string.
	return useSyncExternalStore(store.subscribe, snapshot, snapshot);
}
