import type { EditorRuntime } from "../state/runtime";
import type { Actions } from "./actions";

// The items Mod+C and Mod+X took; Mod+V puts them at the playhead: a copy
// duplicates, a cut moves (the items stay until pasted, so no command is needed
// to bring back what a delete removed). UI state of the page: it is not project
// data and no other actor sees it.
let clipboard: { ids: readonly string[]; cut: boolean } = {
	ids: [],
	cut: false,
};

// The selection, else the focused item.
export function heldItems(runtime: EditorRuntime): readonly string[] {
	const { selection, focusedItem } = runtime.ui.getState();
	if (selection.items.length > 0) return selection.items;
	return focusedItem === null ? [] : [focusedItem];
}

// Copy or cut: false when nothing is held.
export function takeToClipboard(runtime: EditorRuntime, cut: boolean): boolean {
	const held = heldItems(runtime);
	if (held.length === 0) return false;
	clipboard = { ids: [...held], cut };
	return true;
}

// Paste at the playhead: false when the clipboard is empty.
export function pasteFromClipboard(
	runtime: EditorRuntime,
	actions: Actions,
): boolean {
	if (clipboard.ids.length === 0) return false;
	const { ids, cut } = clipboard;
	if (cut) clipboard = { ids: [], cut: false };
	void actions.pasteItems(ids, runtime.ui.getState().playheadFrame, cut);
	return true;
}

// Mod+A and the menu: every item of the project.
export function selectAllItems(runtime: EditorRuntime): void {
	const doc = runtime.documents.getState().document;
	if (doc !== null) runtime.uiWriter.select(Object.keys(doc.items));
}
