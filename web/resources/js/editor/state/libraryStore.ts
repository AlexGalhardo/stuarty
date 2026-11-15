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
