// The keyboard map of SDD-012 section 18.2 that is shared by more than one
// component. The Prototype shortcuts stay in timeline/useShortcuts.ts; every
// key here calls an action of the registry or opens a surface of the editor.

import type { Actions } from "./actions";

// `Mod` is Ctrl on Windows and Linux and Cmd on macOS.
export function hasMod(
	event: Pick<KeyboardEvent, "ctrlKey" | "metaKey">,
): boolean {
	return event.ctrlKey || event.metaKey;
}

// Mod+K opens and closes the command palette, also inside an editable field.
export function isPaletteShortcut(
	event: Pick<
		KeyboardEvent,
		"key" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey"
	>,
): boolean {
	return (
		hasMod(event) &&
		!event.altKey &&
		!event.shiftKey &&
		event.key.toLowerCase() === "k"
	);
}

export type PanelKey = "library" | "inspector" | "history";

const PANEL_KEYS: Readonly<Record<string, PanelKey>> = {
	"1": "library",
	"2": "inspector",
	"3": "history",
};

// Alt+1, Alt+2 and Alt+3 toggle the library, inspector and history panels
// (SDD-012 section 18.2). Alt alone: with Ctrl or Cmd the browser owns it.
export function panelShortcut(
	event: Pick<
		KeyboardEvent,
		"key" | "code" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey"
	>,
): PanelKey | null {
	if (!event.altKey || hasMod(event) || event.shiftKey) return null;
	// `key` is a dead or special character with Option on some keyboards.
	const digit = /^Digit([123])$/.exec(event.code)?.[1] ?? event.key;
	return PANEL_KEYS[digit] ?? null;
}

export type EditKey = "delete" | "ripple_delete" | "duplicate" | "rename";

type EditKeyEvent = Pick<
	KeyboardEvent,
	"key" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey"
>;

// Delete and Backspace delete, Shift+Delete ripples on the track, Mod+D
// duplicates and F2 renames (SDD-012 sections 7.10 and 18.2).
export function editShortcut(event: EditKeyEvent): EditKey | null {
	if (event.altKey) return null;
	if (hasMod(event)) {
		return event.key.toLowerCase() === "d" && !event.shiftKey
			? "duplicate"
			: null;
	}
	if (event.key === "Delete")
		return event.shiftKey ? "ripple_delete" : "delete";
	if (event.key === "Backspace" && !event.shiftKey) return "delete";
	if (event.key === "F2" && !event.shiftKey) return "rename";
	return null;
}

// Mod+A selects every item (SDD-012 section 18.2). Outside an editable field
// only: the field's own select-all wins there.
export function isSelectAllShortcut(event: EditKeyEvent): boolean {
	return (
		hasMod(event) &&
		!event.altKey &&
		!event.shiftKey &&
		event.key.toLowerCase() === "a"
	);
}

// F6 jumps to the next region, Shift+F6 to the previous one.
export function landmarkShortcut(
	event: Pick<
		KeyboardEvent,
		"key" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey"
	>,
): 1 | -1 | null {
	if (event.key !== "F6" || event.altKey || hasMod(event)) return null;
	return event.shiftKey ? -1 : 1;
}

// The table of SDD-012 section 18.2: every row names the action it calls. An
// id that is a method of `Actions` (commands/actions.ts) is checked by the
// compiler; `surface.*` ids are the editor's own surfaces and are listed in
// UI_SHORTCUT_ACTIONS. keymap.test.ts checks both.
export const UI_SHORTCUT_ACTIONS = [
	"surface.play_pause",
	"surface.step_frame",
	"surface.step_second",
	"surface.go_to_edge",
	"surface.zoom",
	"surface.toggle_snap",
	"surface.palette",
	"surface.toggle_panel",
	"surface.select_all",
	"surface.cancel",
	"surface.next_landmark",
	"surface.grid_keys",
] as const;

export type UiShortcutAction = (typeof UI_SHORTCUT_ACTIONS)[number];

export interface Shortcut {
	keys: string;
	action: keyof Actions | UiShortcutAction;
}

export const SHORTCUTS: readonly Shortcut[] = [
	{ keys: "Space", action: "surface.play_pause" },
	{ keys: "Left, Right", action: "surface.step_frame" },
	{ keys: "Shift+Left, Shift+Right", action: "surface.step_second" },
	{ keys: "Home, End", action: "surface.go_to_edge" },
	{ keys: "S", action: "splitAtPlayhead" },
	{ keys: "[", action: "trimToPlayhead" },
	{ keys: "]", action: "trimToPlayhead" },
	{ keys: "Mod+Z", action: "undo" },
	{ keys: "Mod+Shift+Z", action: "redo" },
	{ keys: "Ctrl+Y", action: "redo" },
	{ keys: "+, -, Shift+Z", action: "surface.zoom" },
	{ keys: "Delete, Backspace", action: "deleteSelection" },
	{ keys: "Shift+Delete", action: "deleteSelection" },
	{ keys: "Mod+D", action: "duplicateSelection" },
	{ keys: "C", action: "addCaption" },
	{ keys: "T", action: "addText" },
	{ keys: "F2", action: "renameItem" },
	{ keys: "N", action: "surface.toggle_snap" },
	{ keys: "Mod+K", action: "surface.palette" },
	{ keys: "Alt+1, Alt+2, Alt+3", action: "surface.toggle_panel" },
	{ keys: "Mod+A", action: "surface.select_all" },
	{ keys: "Esc", action: "surface.cancel" },
	{ keys: "F6", action: "surface.next_landmark" },
	{ keys: "Timeline grid keys", action: "surface.grid_keys" },
];
