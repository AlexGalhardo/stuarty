import type { MediaEntry } from "../api/types";
import { createStore, type ReadStore, readOnly } from "./createStore";

// The media library list (SDD-012 section 13.2): what `listMedia` answered,
// patched by `asset.updated` events. Never the document: whether an asset can
// be placed is read from the read model.
export interface LibraryState {
	entries: readonly MediaEntry[];
	loaded: boolean;
	// Cards the user dismissed; UI state only, never persisted.
	dismissed: readonly string[];
	// The last import answer that failed as a whole (E_PATH_NOT_ALLOWED, ...).
	// The path each imported file came from, by file name, for "Try again".
	paths: Readonly<Record<string, string>>;
	importError: { code: string; message: string; hint?: string } | null;
}

export interface LibraryWriter {
	setEntries(entries: readonly MediaEntry[]): void;
	patch(
		assetId: string,
		patch: { state: MediaEntry["state"]; progress: number },
	): void;
	dismiss(assetId: string): void;
	rememberPath(name: string, path: string): void;
	setImportError(error: LibraryState["importError"]): void;
}

const namesOf = new WeakMap<
	readonly MediaEntry[],
	ReadonlyMap<string, string>
>();

// The name the library shows each file under, by asset id: a rename changes it and
// not the document (SDD-011 section 14.8). One map per list, so that a timeline of
// many clips does not walk the list for each of them.
export function shownNames(
	entries: readonly MediaEntry[],
): ReadonlyMap<string, string> {
	let names = namesOf.get(entries);
	if (names === undefined) {
		names = new Map(entries.map((entry) => [entry.asset_id, entry.name]));
		namesOf.set(entries, names);
	}
	return names;
}

export function createLibraryStore(): {
	store: ReadStore<LibraryState>;
	writer: LibraryWriter;
} {
	const store = createStore<LibraryState>({
		entries: [],
		loaded: false,
		dismissed: [],
		paths: {},
		importError: null,
	});
	return {
		store: readOnly(store),
		writer: {
			setEntries: (entries) =>
				store.setState((s) => ({ ...s, entries, loaded: true })),
			patch: (assetId, patch) =>
				store.setState((s) => ({
					...s,
					entries: s.entries.map((entry) =>
						entry.asset_id === assetId ? { ...entry, ...patch } : entry,
					),
				})),
			dismiss: (assetId) =>
				store.setState((s) => ({ ...s, dismissed: [...s.dismissed, assetId] })),
			rememberPath: (name, path) =>
				store.setState((s) => ({ ...s, paths: { ...s.paths, [name]: path } })),
			setImportError: (importError) =>
				store.setState((s) => ({ ...s, importError })),
		},
	};
}
