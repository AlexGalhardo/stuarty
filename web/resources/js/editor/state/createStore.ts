import {
	createStore as createVanillaStore,
	type StoreApi,
	useStore as useZustandStore,
} from "zustand";

// One zustand vanilla store per concern (SDD-012 section 5.1). Components get
// the read-only side through ReadStore; only the module that creates a store
// keeps its setState.
export type ReadStore<S> = Pick<
	StoreApi<S>,
	"getState" | "getInitialState" | "subscribe"
>;

export interface Store<S> extends ReadStore<S> {
	setState(update: (state: S) => S): void;
}

export function createStore<S>(initial: S): Store<S> {
	return createVanillaStore<S>()(() => initial);
}

// The read-only view of a store. The editor has no server render, so the
// "initial" state that React's server snapshot uses is the current state; the
// tests render to a string and need that.
export function readOnly<S>(store: Store<S>): ReadStore<S> {
	return {
		getState: store.getState,
		getInitialState: store.getState,
		subscribe: store.subscribe,
	};
}

// useStore subscribes a component to a slice. The selector must return a
// referentially stable value for an unchanged state (a field, not a new
// object).
export function useStore<S, T>(
	store: ReadStore<S>,
	select: (state: S) => T,
): T {
	return useZustandStore(store, select);
}
