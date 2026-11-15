import { createStore, type ReadStore, readOnly } from "./createStore";

// UI state, never sent to the core (SDD-012 section 5.1): selection, playhead,
// zoom and scroll. The Prototype subset.
export interface UiState {
	// `track` is the track whose panel the inspector shows, when no item is
	// selected (SDD-012 section 12.1).
	selection: { items: readonly string[]; track: string | null };
	playheadFrame: number;
	timeline: { pxPerFrame: number };
	// The revision at which the core answered E_NOTHING_TO_UNDO or
	// E_NOTHING_TO_REDO; the button stays disabled until the next batch.
	historyBlocked: { undo: number | null; redo: number | null };
}

export interface UiWriter {
	select(items: readonly string[]): void;
	selectTrack(trackId: string | null): void;
	dropSelection(isKnown: (id: string) => boolean): void;
	setPlayhead(frame: number): void;
	setZoom(pxPerFrame: number): void;
	blockHistory(kind: "undo" | "redo", revision: number): void;
}

export function createUiStore(initialZoom = 1): {
	store: ReadStore<UiState>;
	writer: UiWriter;
} {
	const store = createStore<UiState>({
		selection: { items: [], track: null },
		playheadFrame: 0,
		timeline: { pxPerFrame: initialZoom },
		historyBlocked: { undo: null, redo: null },
	});
	return {
		store: readOnly(store),
		writer: {
			select: (items) =>
				store.setState((s) => ({ ...s, selection: { items, track: null } })),
			selectTrack: (track) =>
				store.setState((s) => ({
					...s,
					selection: { items: [], track },
				})),
			// The read model removed an id: drop it from the selection (SDD-012 7.6).
			dropSelection(isKnown) {
				store.setState((s) => {
					const kept = s.selection.items.filter(isKnown);
					return kept.length === s.selection.items.length
						? s
						: { ...s, selection: { ...s.selection, items: kept } };
				});
			},
			setPlayhead: (playheadFrame) =>
				store.setState((s) => ({ ...s, playheadFrame })),
			blockHistory: (kind, revision) =>
				store.setState((s) => ({
					...s,
					historyBlocked: { ...s.historyBlocked, [kind]: revision },
				})),
			setZoom: (pxPerFrame) =>
				store.setState((s) => ({
					...s,
					timeline: { ...s.timeline, pxPerFrame },
				})),
		},
	};
}
