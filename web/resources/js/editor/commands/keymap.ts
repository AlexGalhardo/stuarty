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

export type EditKey =
	| "delete"
	| "ripple_delete"
	| "duplicate"
	| "rename"
	| "toggle_enabled"
	| "toggle_link"
	| "toggle_magnet"
	| "extract_audio"
	| "copy_attributes"
	| "paste_attributes";

type EditKeyEvent = Pick<
	KeyboardEvent,
	"key" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey"
>;

// Delete and Backspace delete, Shift+Delete ripples on the track, Mod+D
// duplicates, F2 renames, V turns the item off or on, ~ links or unlinks and
// Mod+Shift+S extracts the audio of a clip and P turns the magnet of the
// track on or off (SDD-012 sections 7.10 and 18.2).
export function editShortcut(event: EditKeyEvent): EditKey | null {
	if (event.altKey) return null;
	if (!hasMod(event) && event.key === "~") return "toggle_link";
	if (!hasMod(event) && !event.shiftKey && event.key.toLowerCase() === "p") {
		return "toggle_magnet";
	}
	if (hasMod(event) && event.shiftKey && event.key.toLowerCase() === "c") {
		return "copy_attributes";
	}
	if (hasMod(event) && event.shiftKey && event.key.toLowerCase() === "v") {
		return "paste_attributes";
	}
	if (hasMod(event) && event.shiftKey && event.key.toLowerCase() === "s") {
		return "extract_audio";
	}
	if (!hasMod(event) && !event.shiftKey && event.key.toLowerCase() === "v") {
		return "toggle_enabled";
	}
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
	"surface.next_cut",
	"surface.shuttle",
	"surface.range",
	"surface.tool",
	"surface.keyframe_panel",
	"surface.speed_curve",
	"surface.scroll",
	"surface.next_marker",
	"surface.zoom",
	"surface.toggle_snap",
	"surface.palette",
	"surface.find",
	"surface.toggle_panel",
	"surface.select_all",
	"surface.select_side",
	"surface.clipboard",
	"surface.preview_zoom",
	"surface.import",
	"surface.export",
	"surface.saved",
	"surface.cancel",
	"surface.next_landmark",
	"surface.grid_keys",
	"surface.cheat_sheet",
] as const;

export type UiShortcutAction = (typeof UI_SHORTCUT_ACTIONS)[number];

// The rows the person can rebind (SDD-012 section 18.2). Their `id` in the
// table is one of these.
export const BINDING_IDS = [
	"split",
	"trim_start",
	"trim_end",
	"add_caption",
	"add_text",
	"toggle_snap",
	"zoom_in",
	"zoom_out",
	"zoom_fit",
] as const;

export type BindingId = (typeof BINDING_IDS)[number];

export interface Shortcut {
	// Names the row in the cheat sheet (ui.editor.shortcuts.rows.<id>).
	id: string;
	keys: string;
	// Keys that do the same and are not listed as a row of their own (the
	// section 18.7 aliases `+` and `-`); other rows may not rebind to them.
	aliases?: string;
	action: keyof Actions | UiShortcutAction;
}

export const SHORTCUTS: readonly Shortcut[] = [
	{ id: "play_pause", keys: "Space", action: "surface.play_pause" },
	{ id: "step_frame", keys: "Left, Right", action: "surface.step_frame" },
	{
		id: "step_second",
		keys: "Shift+Left, Shift+Right",
		action: "surface.step_second",
	},
	{ id: "go_to_edge", keys: "Home, End", action: "surface.go_to_edge" },
	{ id: "next_cut", keys: "Up, Down", action: "surface.next_cut" },
	{ id: "shuttle", keys: "J, K, L", action: "surface.shuttle" },
	{
		id: "next_marker",
		keys: "Shift+M, Alt+Shift+M, Mod+Shift+Left, Mod+Shift+Right",
		action: "surface.next_marker",
	},
	{ id: "marker", keys: "M", action: "toggleMarkerAtPlayhead" },
	{ id: "range_in_out", keys: "I, O", action: "surface.range" },
	{ id: "range_select", keys: "Shift+X", action: "surface.range" },
	{ id: "range_clear", keys: "Alt+X, Mod+Shift+X", action: "surface.range" },
	{ id: "tool", keys: "B, A", action: "surface.tool" },
	{ id: "split", keys: "Mod+B", action: "splitAtPlayhead" },
	{ id: "split_all", keys: "Mod+Shift+B", action: "splitAllAtPlayhead" },
	{ id: "trim_start", keys: "Q", action: "trimToPlayhead" },
	{ id: "trim_end", keys: "W", action: "trimToPlayhead" },
	{
		id: "slide",
		keys: "Mod+Alt+Left, Mod+Alt+Right",
		action: "slideItems",
	},
	{
		id: "roll",
		keys: "Mod+Alt+Shift+Left, Mod+Alt+Shift+Right",
		action: "rollItems",
	},
	{ id: "slip", keys: "Alt+,, Alt+.", action: "slipItems" },
	{ id: "select_side", keys: "[, ]", action: "surface.select_side" },
	{ id: "undo", keys: "Mod+Z", action: "undo" },
	{ id: "redo", keys: "Mod+Shift+Z", action: "redo" },
	{ id: "redo_alt", keys: "Ctrl+Y", action: "redo" },
	{
		id: "zoom_in",
		keys: "Mod+=",
		aliases: "+, Mod++",
		action: "surface.zoom",
	},
	{ id: "zoom_out", keys: "Mod+-", aliases: "-", action: "surface.zoom" },
	{ id: "zoom_fit", keys: "Shift+Z", action: "surface.zoom" },
	{ id: "zoom_wheel", keys: "Mod+Wheel", action: "surface.zoom" },
	{ id: "delete", keys: "Delete, Backspace", action: "deleteSelection" },
	{ id: "ripple_delete", keys: "Shift+Delete", action: "deleteSelection" },
	{ id: "duplicate", keys: "Mod+D", action: "duplicateSelection" },
	{ id: "copy", keys: "Mod+C", action: "surface.clipboard" },
	{ id: "cut", keys: "Mod+X", action: "surface.clipboard" },
	{ id: "paste", keys: "Mod+V", action: "surface.clipboard" },
	{ id: "volume_up", keys: "Mod+.", action: "adjustVolume" },
	{ id: "volume_down", keys: "Mod+,", action: "adjustVolume" },
	{ id: "add_caption", keys: "C", action: "addCaption" },
	{ id: "add_text", keys: "T", action: "addText" },
	{ id: "rename", keys: "F2", action: "renameItem" },
	{ id: "toggle_enabled", keys: "V", action: "toggleEnabled" },
	{ id: "link", keys: "~", action: "toggleLink" },
	{ id: "extract_audio", keys: "Mod+Shift+S", action: "extractAudio" },
	{ id: "toggle_magnet", keys: "P", action: "toggleMagnet" },
	{ id: "copy_attributes", keys: "Mod+Shift+C", action: "copyAttributes" },
	{ id: "paste_attributes", keys: "Mod+Shift+V", action: "pasteAttributes" },
	{ id: "toggle_snap", keys: "N", action: "surface.toggle_snap" },
	{ id: "keyframe_panel", keys: "Alt+K", action: "surface.keyframe_panel" },
	{ id: "speed_curve", keys: "Shift+B", action: "surface.speed_curve" },
	{ id: "scroll", keys: "Wheel, Alt+Wheel", action: "surface.scroll" },
	{
		id: "preview_zoom",
		keys: "Alt+Shift+Z, Alt+Shift+=, Alt+Shift+-",
		action: "surface.preview_zoom",
	},
	{ id: "import", keys: "Mod+I", action: "surface.import" },
	{ id: "export", keys: "Mod+E", action: "surface.export" },
	{ id: "save", keys: "Mod+S", action: "surface.saved" },
	{ id: "palette", keys: "Mod+K", action: "surface.palette" },
	{ id: "find", keys: "Mod+F", action: "surface.find" },
	{
		id: "toggle_panel",
		keys: "Alt+1, Alt+2, Alt+3",
		action: "surface.toggle_panel",
	},
	{ id: "select_all", keys: "Mod+A", action: "surface.select_all" },
	{ id: "cancel", keys: "Esc", action: "surface.cancel" },
	{ id: "next_landmark", keys: "F6", action: "surface.next_landmark" },
	{ id: "grid_keys", keys: "Timeline grid keys", action: "surface.grid_keys" },
	{ id: "cheat_sheet", keys: "?", action: "surface.cheat_sheet" },
];

// The row of the table that holds a key, for the fixed rows (the rebindable
// ones are read through their bindings, keymapStore.ts).
const FIXED_ROW_BY_COMBO: ReadonlyMap<string, string> = new Map(
	SHORTCUTS.filter(
		(row) => !(BINDING_IDS as readonly string[]).includes(row.id),
	).flatMap((row) => row.keys.split(", ").map((key) => [key, row.id] as const)),
);

export function fixedRowFor(combo: string): string | null {
	return FIXED_ROW_BY_COMBO.get(combo) ?? null;
}

export type Bindings = Readonly<Record<BindingId, string>>;

export function isBindingId(id: string): id is BindingId {
	return (BINDING_IDS as readonly string[]).includes(id);
}

export const DEFAULT_BINDINGS: Bindings = Object.fromEntries(
	BINDING_IDS.map((id) => [
		id,
		SHORTCUTS.find((row) => row.id === id)?.keys ?? "",
	]),
) as Bindings;

// Keys that no binding may take: Tab and Enter belong to focus and forms.
export const RESERVED_KEYS: readonly string[] = ["Tab", "Shift+Tab", "Enter"];

const KEY_NAMES: Readonly<Record<string, string>> = {
	" ": "Space",
	ArrowLeft: "Left",
	ArrowRight: "Right",
	ArrowUp: "Up",
	ArrowDown: "Down",
	Escape: "Esc",
};

const MODIFIER_KEYS = new Set(["Control", "Meta", "Alt", "Shift", "AltGraph"]);

type ComboEvent = Pick<
	KeyboardEvent,
	"key" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey"
>;

// The canonical text of a key press, in the form of the table: `Mod`, `Alt`,
// `Shift` and the key. Shift is part of it for a letter or a named key only:
// for `+` or `?` the character already says it. Null for a modifier alone.
export function comboFromEvent(event: ComboEvent): string | null {
	if (MODIFIER_KEYS.has(event.key)) return null;
	const name = KEY_NAMES[event.key] ?? event.key;
	const single = name.length === 1;
	const letter = single && name.toLowerCase() !== name.toUpperCase();
	const parts: string[] = [];
	if (hasMod(event)) parts.push("Mod");
	if (event.altKey) parts.push("Alt");
	if (event.shiftKey && (letter || !single)) parts.push("Shift");
	parts.push(letter ? name.toUpperCase() : name);
	return parts.join("+");
}

// The text of a combo for the person: Mod is Ctrl or Cmd by platform.
export function displayCombo(combo: string, mac: boolean): string {
	return combo.replace(/\bMod\b/g, mac ? "Cmd" : "Ctrl");
}

export const IS_MAC: boolean =
	typeof navigator !== "undefined" &&
	/Mac|iPhone|iPad/.test(navigator.userAgent);

// The first key of the row that is current now, as shown ("Ctrl+B"), or null
// when the row does not exist or has no key.
export function keyLabel(
	id: string,
	bindings: Bindings,
	mac: boolean,
): string | null {
	const row = SHORTCUTS.find((candidate) => candidate.id === id);
	if (row === undefined) return null;
	const keys = isBindingId(id) ? bindings[id] : row.keys.split(", ")[0];
	return keys === undefined || keys === "" ? null : displayCombo(keys, mac);
}

// "Split (Ctrl+B)": the label with the first key of the row that is current
// now, so a tooltip never states a key that was rebound. A row that does not
// exist, or has no key, leaves the label alone.
export function labelWithKey(
	label: string,
	id: string,
	bindings: Bindings,
	mac: boolean,
): string {
	const key = keyLabel(id, bindings, mac);
	return key === null ? label : `${label} (${key})`;
}
