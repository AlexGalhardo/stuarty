import type { ApiClient } from "../api/client";
import { ApiError } from "../api/errors";
import type { Timers } from "../api/events";
import type { HistoryEntry, ProjectEvent } from "../api/types";
import { createStore, type ReadStore, readOnly } from "./createStore";

// One line of the history panel (SDD-012 section 15). A row that came from a
// `batch.committed` event is provisional: it has only what the event carries.
export interface HistoryRow {
	revision: number;
	actor: string;
	at: string;
	kind: string;
	// The description of the event, English, shown as received.
	description?: string;
	// What the page of the history says instead of a description.
	ops?: readonly string[];
	changes?: number;
	undone: boolean;
	provisional: boolean;
}

export interface HistoryState {
	status: "idle" | "loading" | "ready" | "failed";
	rows: readonly HistoryRow[];
	nextCursor: string | null;
	loadingMore: boolean;
	error: ApiError | null;
	// From the first page only (SDD-012 section 15): null while no row says it.
	undoable: boolean | null;
	redoable: boolean | null;
}

export const PAGE_SIZE = 50;
export const REFETCH_DEBOUNCE_MS = 250;

export interface HistoryController {
	store: ReadStore<HistoryState>;
	// The first page, replacing the provisional rows; every page after it that
	// was loaded stays.
	refresh(): Promise<void>;
	loadMore(): Promise<void>;
	// A batch.committed event: prepend a provisional row, refetch in 250 ms.
	onBatch(event: Extract<ProjectEvent, { name: "batch.committed" }>): void;
	// The stream lost events: one refetch now.
	onGap(): void;
	dispose(): void;
}

function fromEntry(entry: HistoryEntry): HistoryRow {
	return {
		revision: entry.revision,
		actor: entry.actor,
		at: entry.at,
		kind: entry.kind,
		ops: entry.ops,
		changes: entry.changes,
		undone: entry.undone_by !== null && entry.undone_by !== undefined,
		provisional: false,
	};
}

// The flag is true on the one step Undo or Redo would take for the caller.
function flag(
	entries: readonly HistoryEntry[],
	key: "undoable_by_me" | "redoable_by_me",
): boolean {
	return entries.some((entry) => entry[key]);
}

export function createHistory(deps: {
	client: ApiClient;
	projectId: string;
	timers: Timers;
}): HistoryController {
	const { client, projectId, timers } = deps;
	const store = createStore<HistoryState>({
		status: "idle",
		rows: [],
		nextCursor: null,
		loadingMore: false,
		error: null,
		undoable: null,
		redoable: null,
	});
	let timer: unknown;
	let disposed = false;
	let generation = 0;

	async function refresh(): Promise<void> {
		const mine = ++generation;
		if (store.getState().status === "idle")
			store.setState((s) => ({ ...s, status: "loading" }));
		try {
			const page = await client.getHistory(projectId, { limit: PAGE_SIZE });
			if (disposed || mine !== generation) return;
			const first = page.entries.map(fromEntry);
			const oldest = first.length === 0 ? 0 : (first.at(-1)?.revision ?? 0);
			store.setState((s) => {
				// Rows of older pages already loaded stay; so does the cursor that
				// goes with them.
				const older = s.rows.filter(
					(row) => !row.provisional && row.revision < oldest,
				);
				return {
					status: "ready",
					rows: [...first, ...older],
					nextCursor:
						older.length > 0 && s.nextCursor !== null
							? s.nextCursor
							: page.next_cursor,
					loadingMore: false,
					error: null,
					undoable: flag(page.entries, "undoable_by_me"),
					redoable: flag(page.entries, "redoable_by_me"),
				};
			});
		} catch (error) {
			if (disposed || mine !== generation || !(error instanceof ApiError))
				return;
			store.setState((s) => ({ ...s, status: "failed", error }));
		}
	}

	return {
		store: readOnly(store),
		refresh,
		async loadMore() {
			const { nextCursor, loadingMore } = store.getState();
			if (nextCursor === null || loadingMore) return;
			store.setState((s) => ({ ...s, loadingMore: true }));
			try {
				const page = await client.getHistory(projectId, {
					cursor: nextCursor,
					limit: PAGE_SIZE,
				});
				if (disposed) return;
				store.setState((s) => {
					const known = new Set(s.rows.map((row) => row.revision));
					return {
						...s,
						rows: [
							...s.rows,
							...page.entries
								.map(fromEntry)
								.filter((row) => !known.has(row.revision)),
						],
						nextCursor: page.next_cursor,
						loadingMore: false,
					};
				});
			} catch (error) {
				if (disposed || !(error instanceof ApiError)) return;
				store.setState((s) => ({ ...s, loadingMore: false, error }));
			}
		},
		onBatch(event) {
			if (store.getState().status === "idle") return;
			store.setState((s) => {
				if (s.rows.some((row) => row.revision >= event.revision)) return s;
				const row: HistoryRow = {
					revision: event.revision,
					actor: event.actor,
					at: event.at,
					kind: event.kind,
					description: event.description,
					undone: false,
					provisional: true,
				};
				return { ...s, rows: [row, ...s.rows] };
			});
			timers.clearTimeout(timer);
			timer = timers.setTimeout(() => void refresh(), REFETCH_DEBOUNCE_MS);
		},
		onGap() {
			if (store.getState().status === "idle") return;
			timers.clearTimeout(timer);
			void refresh();
		},
		dispose() {
			disposed = true;
			timers.clearTimeout(timer);
		},
	};
}
