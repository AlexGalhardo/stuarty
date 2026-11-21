import { createStore, type ReadStore, readOnly } from "./createStore";
import { ROW_BOUNDS, readLayout, writeLayout } from "./prefs";

// The size of the preview canvas (SDD-012 section 11.4): fitted to the region,
// or half or all of the pixel buffer.
export type PreviewSize = "fit" | "50" | "100";

// A keyframe as the read model stores it: `t` is relative to the item start.
export interface KeyframeRef {
	item: string;
	property: string;
	t: number;
}

// UI state, never sent to the core (SDD-012 section 5.1): selection, playhead,
// zoom and scroll. The Prototype subset.
export interface UiState {
	// `track` is the track whose panel the inspector shows, when no item is
	// selected (SDD-012 section 12.1).
	// `anchor` is the item a Shift+click or Shift+arrow range starts from.
	// `keyframe` is the diamond or list row selected inside the selected item.
	selection: {
		items: readonly string[];
		track: string | null;
		anchor: string | null;
		keyframe: KeyframeRef | null;
	};
	// The item that holds the roving tab stop of the timeline grid (SDD-012 7.10).
	focusedItem: string | null;
	// The item whose name is being edited inline (F2, the menu), if any.
	renamingItem: string | null;
	// The item whose Repeat dialog is open (`item.repeat`), if any.
	repeatingItem: string | null;
	// The caption word that is selected and the one being edited inline; the
	// item must also be selected for the word to show as selected.
	selectedWord: { item: string; index: number } | null;
	editingWord: { item: string; index: number } | null;
	// The text item whose content is edited inline over the canvas (SDD-012
	// section 11.6), if any.
	editingText: string | null;
	// The safe-area guide over the preview, off by default (SDD-012 section 11.6).
	safeAreaVisible: boolean;
	// The eyedropper of VV-186: the item whose chroma key colour the next click
	// on the preview picks, and what receives the colour (the inspector sends it
	// through the existing `clip.set_chroma_key`); null when it is off.
	eyedropper: { item: string; onPick(hex: string): void } | null;
	// E_IN_USE of an asset.remove: the ids `details.used_by` named, shown by the
	// dialog (SDD-012 section 4.4); null when the dialog is closed.
	inUse: { assetId: string; usedBy: readonly string[] } | null;
	// The timeline tool of B and A (SDD-012 section 18.7): the blade cuts the
	// item that is clicked at the pointer; the select tool is the usual one.
	tool: "select" | "blade";
	playheadFrame: number;
	// The in and out points of I and O (VV-035): the span a later export or loop
	// takes. Screen state of this page; the core does not know them.
	range: { in: number | null; out: number | null };
	timeline: { pxPerFrame: number; snapEnabled: boolean };
	// Track row heights in pixels by kind of track, persisted in `vv.layout`.
	rowPx: { visual: number; audio: number };
	// The preview size and the preview audio level. Both are UI state only:
	// they never reach the core (SDD-012 sections 11.4 and 11.5).
	previewSize: PreviewSize;
	previewVolume: number;
	previewMuted: boolean;
	// The revision at which the core answered E_NOTHING_TO_UNDO or
	// E_NOTHING_TO_REDO; the button stays disabled until the next batch.
	historyBlocked: { undo: number | null; redo: number | null };
	// The tab of the left panel and which panels are shown (SDD-012 sections
	// 6.1 and 18.2: Alt+1, Alt+2 and Alt+3).
	activePanelTab: LeftTab;
	panels: { left: boolean; inspector: boolean; history: boolean };
	inspectorTab: InspectorTab;
}

// The tabs of the material rail (SDD-012 section 6.4); "library" is Media.
export type LeftTab =
	| "library"
	| "audio"
	| "text"
	| "captions"
	| "effects"
	| "transitions"
	| "filters"
	| "templates";

// The tab strip of the inspector (SDD-012 section 6.4). It is remembered across
// selections; a selection that lacks the tab shows its first one.
export type InspectorTab =
	| "video"
	| "audio"
	| "speed"
	| "animation"
	| "adjust"
	| "text";

export interface UiWriter {
	select(items: readonly string[], anchor?: string | null): void;
	// Selects one keyframe, and so its item (SDD-012 section 12.1, Delete).
	selectKeyframe(keyframe: KeyframeRef | null): void;
	focusItem(id: string | null): void;
	startRename(id: string | null): void;
	startRepeat(id: string | null): void;
	startTextEdit(id: string | null): void;
	selectWord(word: UiState["selectedWord"]): void;
	startWordEdit(word: UiState["editingWord"]): void;
	toggleSafeArea(): void;
	setEyedropper(eyedropper: UiState["eyedropper"]): void;
	showInUse(inUse: UiState["inUse"]): void;
	selectTrack(trackId: string | null): void;
	dropSelection(isKnown: (id: string) => boolean): void;
	setTool(tool: UiState["tool"]): void;
	setPlayhead(frame: number): void;
	setRangeIn(frame: number): void;
	setRangeOut(frame: number): void;
	clearRange(): void;
	setZoom(pxPerFrame: number): void;
	toggleSnap(): void;
	setRowPx(kind: "visual" | "audio", px: number): void;
	setPreviewSize(size: PreviewSize): void;
	setPreviewVolume(volume: number): void;
	togglePreviewMuted(): void;
	blockHistory(kind: "undo" | "redo", revision: number): void;
	// Alt+3: open or collapse the history sidebar; the choice is remembered.
	toggleHistory(): void;
	toggleInspector(): void;
	showInspectorTab(tab: InspectorTab): void;
	showLeftTab(tab: LeftTab): void;
	// Alt+1: show or hide the whole left region (the rail stays); the last tab comes back.
	toggleLeftRegion(): void;
}

export function createUiStore(initialZoom = 1): {
	store: ReadStore<UiState>;
	writer: UiWriter;
} {
	const store = createStore<UiState>({
		selection: { items: [], track: null, anchor: null, keyframe: null },
		focusedItem: null,
		renamingItem: null,
		repeatingItem: null,
		editingText: null,
		selectedWord: null,
		editingWord: null,
		safeAreaVisible: false,
		eyedropper: null,
		inUse: null,
		tool: "select",
		playheadFrame: 0,
		range: { in: null, out: null },
		timeline: { pxPerFrame: initialZoom, snapEnabled: true },
		rowPx: readLayout().rows,
		previewSize: "fit",
		previewVolume: 1,
		previewMuted: false,
		historyBlocked: { undo: null, redo: null },
		activePanelTab: "library",
		panels: { left: true, inspector: true, history: readLayout().historyOpen },
		inspectorTab: "video",
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
						keyframe: null,
					},
				})),
			selectKeyframe: (keyframe) =>
				store.setState((s) =>
					keyframe === null
						? { ...s, selection: { ...s.selection, keyframe: null } }
						: {
								...s,
								selection: {
									items: [keyframe.item],
									track: null,
									anchor: keyframe.item,
									keyframe,
								},
							},
				),
			focusItem: (focusedItem) =>
				store.setState((s) =>
					s.focusedItem === focusedItem ? s : { ...s, focusedItem },
				),
			startRename: (renamingItem) =>
				store.setState((s) =>
					s.renamingItem === renamingItem ? s : { ...s, renamingItem },
				),
			startRepeat: (repeatingItem) =>
				store.setState((s) =>
					s.repeatingItem === repeatingItem ? s : { ...s, repeatingItem },
				),
			selectWord: (selectedWord) =>
				store.setState((s) => ({ ...s, selectedWord })),
			startWordEdit: (editingWord) =>
				store.setState((s) => ({ ...s, editingWord })),
			startTextEdit: (editingText) =>
				store.setState((s) =>
					s.editingText === editingText ? s : { ...s, editingText },
				),
			toggleSafeArea: () =>
				store.setState((s) => ({ ...s, safeAreaVisible: !s.safeAreaVisible })),
			setEyedropper: (eyedropper) =>
				store.setState((s) =>
					s.eyedropper === eyedropper ? s : { ...s, eyedropper },
				),
			showInUse: (inUse) => store.setState((s) => ({ ...s, inUse })),
			selectTrack: (track) =>
				store.setState((s) => ({
					...s,
					selection: { items: [], track, anchor: null, keyframe: null },
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
						: {
								...s,
								selection: {
									...s.selection,
									items: kept,
									anchor,
									keyframe:
										s.selection.keyframe !== null &&
										kept.includes(s.selection.keyframe.item)
											? s.selection.keyframe
											: null,
								},
							};
				});
			},
			setTool: (tool) => store.setState((s) => ({ ...s, tool })),
			setPlayhead: (playheadFrame) =>
				store.setState((s) => ({ ...s, playheadFrame })),
			// An in point after the out point (or the reverse) drops the other end.
			setRangeIn: (frame) =>
				store.setState((s) => ({
					...s,
					range: {
						in: frame,
						out:
							s.range.out !== null && s.range.out <= frame ? null : s.range.out,
					},
				})),
			setRangeOut: (frame) =>
				store.setState((s) => ({
					...s,
					range: {
						in: s.range.in !== null && s.range.in >= frame ? null : s.range.in,
						out: frame,
					},
				})),
			clearRange: () =>
				store.setState((s) =>
					s.range.in === null && s.range.out === null
						? s
						: { ...s, range: { in: null, out: null } },
				),
			blockHistory: (kind, revision) =>
				store.setState((s) => ({
					...s,
					historyBlocked: { ...s.historyBlocked, [kind]: revision },
				})),
			toggleHistory: () => {
				const history = !store.getState().panels.history;
				store.setState((s) => ({ ...s, panels: { ...s.panels, history } }));
				writeLayout({ historyOpen: history });
			},
			toggleLeftRegion: () =>
				store.setState((s) => ({
					...s,
					panels: { ...s.panels, left: !s.panels.left },
				})),
			showLeftTab: (tab) =>
				store.setState((s) => ({
					...s,
					activePanelTab: tab,
					panels: { ...s.panels, left: true },
				})),
			showInspectorTab: (tab) =>
				store.setState((s) =>
					s.inspectorTab === tab ? s : { ...s, inspectorTab: tab },
				),
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
			setRowPx: (kind, px) => {
				const next = Math.round(
					Math.min(Math.max(px, ROW_BOUNDS.min), ROW_BOUNDS.max),
				);
				if (store.getState().rowPx[kind] === next) return;
				const rowPx = { ...store.getState().rowPx, [kind]: next };
				store.setState((s) => ({ ...s, rowPx }));
				writeLayout({ rows: rowPx });
			},
			setPreviewSize: (previewSize) =>
				store.setState((s) => ({ ...s, previewSize })),
			setPreviewVolume: (volume) =>
				store.setState((s) => ({
					...s,
					previewVolume: Math.min(Math.max(volume, 0), 1),
				})),
			togglePreviewMuted: () =>
				store.setState((s) => ({ ...s, previewMuted: !s.previewMuted })),
			setZoom: (pxPerFrame) =>
				store.setState((s) => ({
					...s,
					timeline: { ...s.timeline, pxPerFrame },
				})),
		},
	};
}
