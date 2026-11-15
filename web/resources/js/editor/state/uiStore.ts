import { createStore, type ReadStore, readOnly } from "./createStore";

// UI state, never sent to the core (SDD-012 section 5.1): selection, playhead,
// zoom and scroll. The Prototype subset.
export interface UiState {
	// `track` is the track whose panel the inspector shows, when no item is
	// selected (SDD-012 section 12.1).
	// `anchor` is the item a Shift+click or Shift+arrow range starts from.
	selection: {
		items: readonly string[];
		track: string | null;
		anchor: string | null;
	};
	// The item that holds the roving tab stop of the timeline grid (SDD-012 7.10).
	focusedItem: string | null;
	// The item whose name is being edited inline (F2, the menu), if any.
	renamingItem: string | null;
	// The safe-area guide over the preview, off by default (SDD-012 section 11.6).
	safeAreaVisible: boolean;
	// E_IN_USE of an asset.remove: the ids `details.used_by` named, shown by the
	// dialog (SDD-012 section 4.4); null when the dialog is closed.
	inUse: { assetId: string; usedBy: readonly string[] } | null;
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
	select(items: readonly string[], anchor?: string | null): void;
	focusItem(id: string | null): void;
	startRename(id: string | null): void;
	toggleSafeArea(): void;
	showInUse(inUse: UiState["inUse"]): void;
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
		selection: { items: [], track: null, anchor: null },
		focusedItem: null,
		renamingItem: null,
		safeAreaVisible: false,
		inUse: null,
		playheadFrame: 0,
		timeline: { pxPerFrame: initialZoom, snapEnabled: true },
		historyBlocked: { undo: null, redo: null },
		activePanelTab: "library",
		panels: { left: true, inspector: true },
	});
	return {
		store: readOnly(store),
		writer: {
			select: (items, anchor) =>
				store.setState((s) => ({
					...s,
					selection: {
						items,
						track: null,
						anchor: anchor === undefined ? (items.at(-1) ?? null) : anchor,
					},
				})),
			focusItem: (focusedItem) =>
				store.setState((s) =>
					s.focusedItem === focusedItem ? s : { ...s, focusedItem },
				),
			startRename: (renamingItem) =>
				store.setState((s) =>
					s.renamingItem === renamingItem ? s : { ...s, renamingItem },
				),
			toggleSafeArea: () =>
				store.setState((s) => ({ ...s, safeAreaVisible: !s.safeAreaVisible })),
			showInUse: (inUse) => store.setState((s) => ({ ...s, inUse })),
			selectTrack: (track) =>
				store.setState((s) => ({
					...s,
					selection: { items: [], track, anchor: null },
				})),
			// The read model removed an id: drop it from the selection (SDD-012 7.6).
			dropSelection(isKnown) {
				store.setState((s) => {
					const kept = s.selection.items.filter(isKnown);
					const anchor =
						s.selection.anchor !== null && isKnown(s.selection.anchor)
							? s.selection.anchor
							: (kept.at(-1) ?? null);
					return kept.length === s.selection.items.length &&
						anchor === s.selection.anchor
						? s
						: { ...s, selection: { ...s.selection, items: kept, anchor } };
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
