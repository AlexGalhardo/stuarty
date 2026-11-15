import { createStore, type ReadStore, readOnly } from "./createStore";

// UI state, never sent to the core (SDD-012 section 5.1): selection, playhead,
// zoom and scroll. The Prototype subset.
export interface UiState {
	// `track` is the track whose panel the inspector shows, when no item is
	// selected (SDD-012 section 12.1).
	selection: { items: readonly string[]; track: string | null };
	playheadFrame: number;
	timeline: { pxPerFrame: number; snapEnabled: boolean };
	// The revision at which the core answered E_NOTHING_TO_UNDO or
	// E_NOTHING_TO_REDO; the button stays disabled until the next batch.
	historyBlocked: { undo: number | null; redo: number | null };
	// The tab of the left panel and which panels are shown (SDD-012 sections
	// 6.1 and 18.2: Alt+1, Alt+2 and Alt+3).
	activePanelTab: "library" | "history";
	panels: { left: boolean; inspector: boolean };
}

export interface UiWriter {
	select(items: readonly string[]): void;
	selectTrack(trackId: string | null): void;
	dropSelection(isKnown: (id: string) => boolean): void;
	setPlayhead(frame: number): void;
	setZoom(pxPerFrame: number): void;
	toggleSnap(): void;
	blockHistory(kind: "undo" | "redo", revision: number): void;
	// Show a tab of the left panel; when it is already the shown one, hide the
	// panel (a toggle).
	toggleLeftTab(tab: "library" | "history"): void;
	toggleInspector(): void;
	showLeftTab(tab: "library" | "history"): void;
}

export function createUiStore(initialZoom = 1): {
	store: ReadStore<UiState>;
	writer: UiWriter;
} {
	const store = createStore<UiState>({
		selection: { items: [], track: null },
		playheadFrame: 0,
		timeline: { pxPerFrame: initialZoom, snapEnabled: true },
		historyBlocked: { undo: null, redo: null },
		activePanelTab: "library",
		panels: { left: true, inspector: true },
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
			toggleLeftTab: (tab) =>
				store.setState((s) =>
					s.panels.left && s.activePanelTab === tab
						? { ...s, panels: { ...s.panels, left: false } }
						: {
								...s,
								activePanelTab: tab,
								panels: { ...s.panels, left: true },
							},
				),
			showLeftTab: (tab) =>
				store.setState((s) => ({
					...s,
					activePanelTab: tab,
					panels: { ...s.panels, left: true },
				})),
			toggleInspector: () =>
				store.setState((s) => ({
					...s,
					panels: { ...s.panels, inspector: !s.panels.inspector },
				})),
			toggleSnap: () =>
				store.setState((s) => ({
					...s,
					timeline: { ...s.timeline, snapEnabled: !s.timeline.snapEnabled },
				})),
			setZoom: (pxPerFrame) =>
				store.setState((s) => ({
					...s,
					timeline: { ...s.timeline, pxPerFrame },
				})),
		},
	};
}
