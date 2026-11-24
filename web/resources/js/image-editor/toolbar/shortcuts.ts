// The keys shown in the tooltips of the toolbar (SDD-012 section 24.10). The image editor has a fixed keymap
// (keys.ts), so this table is the one place a tooltip reads its key from: a button names its row, never a
// literal, and the keymap test fails when a row is missing.
export const SHORTCUTS = {
	undo: "Ctrl+Z",
	redo: "Ctrl+Shift+Z",
	group: "Ctrl+G",
	ungroup: "Ctrl+Shift+G",
	duplicate: "Ctrl+D",
	remove: "Delete",
	forward: "]",
	back: "[",
	zoomIn: "+",
	zoomOut: "-",
	fit: "0",
	actual: "1",
} as const;

export type ShortcutRow = keyof typeof SHORTCUTS;
