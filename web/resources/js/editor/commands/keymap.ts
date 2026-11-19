// The shortcut registry of SDD-012 sections 18.2 and 18.9. This file is the only
// place that writes a key: the cheat sheet, the tooltips, the menus, the site
// generator and every handler read the table below (through keymapStore.ts for
// the current keys), and no handler names a key. The file is imported as it is
// by the documentation generator (site/scripts/shortcuts-reference.ts), so it
// has type imports only.

import type { Actions } from "./actions";

// `Mod` is Ctrl on Windows and Linux and Cmd on macOS.
export function hasMod(
	event: Pick<KeyboardEvent, "ctrlKey" | "metaKey">,
): boolean {
	return event.ctrlKey || event.metaKey;
}

// Where a row acts (SDD-012 section 18.9 item 2). `global`: everywhere in the
// editor outside an editable field and an open dialog. `timeline` and
// `preview`: the timeline (and the playhead slider) and the player. `text`:
// also inside an editable field (the palette and the find bar).
export type Scope = "global" | "timeline" | "preview" | "text";

export const PRESET_IDS = ["short", "classic"] as const;
export type PresetId = (typeof PRESET_IDS)[number];
export const DEFAULT_PRESET: PresetId = "short";

// The rows whose action is an editor surface or tool and not a method of the
// action registry. keymap.test.ts checks every row against `Actions` and this
// list.
export const UI_SHORTCUT_ACTIONS = [
	"surface.play_pause",
	"surface.step_frame",
	"surface.step_second",
	"surface.go_to_edge",
	"surface.next_cut",
	"surface.shuttle",
	"surface.next_marker",
	"surface.range",
	"surface.tool",
	"surface.keyframe_panel",
	"surface.speed_curve",
	"surface.scroll",
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

export interface Shortcut {
	// Names the row in the cheat sheet (ui.editor.shortcuts.rows.<id>).
	id: string;
	scope: Scope;
	// The keys of the `short` preset: a list of keys that do the same.
	keys: string;
	// The keys of the `classic` preset, when they differ from `keys`.
	classic?: string;
	// Keys that do the same and are not listed as a row of their own (the
	// section 18.7 aliases `+` and `-`); other rows may not rebind to them.
	aliases?: string;
	action: keyof Actions | UiShortcutAction;
	// A row the person cannot rebind: the wheel, the grid keys, Esc. It is
	// listed and no handler reads a key from it.
	fixed?: true;
}

export const SHORTCUTS: readonly Shortcut[] = [
	{
		id: "play_pause",
		scope: "preview",
		keys: "Space",
		action: "surface.play_pause",
	},
	{
		id: "shuttle_back",
		scope: "preview",
		keys: "J",
		action: "surface.shuttle",
	},
	{ id: "pause", scope: "preview", keys: "K", action: "surface.shuttle" },
	{
		id: "shuttle_play",
		scope: "preview",
		keys: "L",
		action: "surface.shuttle",
	},
	{
		id: "step_back",
		scope: "timeline",
		keys: "Left",
		action: "surface.step_frame",
	},
	{
		id: "step_forward",
		scope: "timeline",
		keys: "Right",
		action: "surface.step_frame",
	},
	{
		id: "step_second_back",
		scope: "timeline",
		keys: "Shift+Left",
		aliases: "PageDown",
		action: "surface.step_second",
	},
	{
		id: "step_second_forward",
		scope: "timeline",
		keys: "Shift+Right",
		aliases: "PageUp",
		action: "surface.step_second",
	},
	{
		id: "go_start",
		scope: "timeline",
		keys: "Home",
		action: "surface.go_to_edge",
	},
	{
		id: "go_end",
		scope: "timeline",
		keys: "End",
		action: "surface.go_to_edge",
	},
	{ id: "prev_cut", scope: "timeline", keys: "Up", action: "surface.next_cut" },
	{
		id: "next_cut",
		scope: "timeline",
		keys: "Down",
		action: "surface.next_cut",
	},
	{
		id: "prev_marker",
		scope: "timeline",
		keys: "Alt+Shift+M, Mod+Shift+Left",
		action: "surface.next_marker",
	},
	{
		id: "next_marker",
		scope: "timeline",
		keys: "Shift+M, Mod+Shift+Right",
		action: "surface.next_marker",
	},
	{
		id: "marker",
		scope: "timeline",
		keys: "M",
		action: "toggleMarkerAtPlayhead",
	},
	{ id: "range_in", scope: "timeline", keys: "I", action: "surface.range" },
	{ id: "range_out", scope: "timeline", keys: "O", action: "surface.range" },
	{
		id: "range_select",
		scope: "timeline",
		keys: "Shift+X",
		action: "surface.range",
	},
	{
		id: "range_clear",
		scope: "timeline",
		keys: "Alt+X, Mod+Shift+X",
		action: "surface.range",
	},
	{
		id: "tool_blade",
		scope: "timeline",
		keys: "B",
		classic: "C",
		action: "surface.tool",
	},
	{
		id: "tool_select",
		scope: "timeline",
		keys: "A",
		classic: "V",
		action: "surface.tool",
	},
	{
		id: "split",
		scope: "timeline",
		keys: "Mod+B",
		classic: "S",
		action: "splitAtPlayhead",
	},
	{
		id: "split_all",
		scope: "timeline",
		keys: "Mod+Shift+B",
		classic: "Shift+S",
		action: "splitAllAtPlayhead",
	},
	{ id: "trim_start", scope: "timeline", keys: "Q", action: "trimToPlayhead" },
	{ id: "trim_end", scope: "timeline", keys: "W", action: "trimToPlayhead" },
	{
		id: "slide_back",
		scope: "timeline",
		keys: "Mod+Alt+Left",
		action: "slideItems",
	},
	{
		id: "slide_forward",
		scope: "timeline",
		keys: "Mod+Alt+Right",
		action: "slideItems",
	},
	{
		id: "roll_back",
		scope: "timeline",
		keys: "Mod+Alt+Shift+Left",
		action: "rollItems",
	},
	{
		id: "roll_forward",
		scope: "timeline",
		keys: "Mod+Alt+Shift+Right",
		action: "rollItems",
	},
	{ id: "slip_back", scope: "timeline", keys: "Alt+,", action: "slipItems" },
	{ id: "slip_forward", scope: "timeline", keys: "Alt+.", action: "slipItems" },
	{
		id: "select_left",
		scope: "timeline",
		keys: "[",
		action: "surface.select_side",
	},
	{
		id: "select_right",
		scope: "timeline",
		keys: "]",
		action: "surface.select_side",
	},
	{ id: "undo", scope: "global", keys: "Mod+Z", action: "undo" },
	{ id: "redo", scope: "global", keys: "Mod+Shift+Z, Mod+Y", action: "redo" },
	{
		id: "zoom_in",
		scope: "timeline",
		keys: "Mod+=",
		aliases: "+, Mod++, =",
		action: "surface.zoom",
	},
	{
		id: "zoom_out",
		scope: "timeline",
		keys: "Mod+-",
		aliases: "-",
		action: "surface.zoom",
	},
	{
		id: "zoom_fit",
		scope: "timeline",
		keys: "Shift+Z",
		classic: "\\",
		action: "surface.zoom",
	},
	{
		id: "zoom_wheel",
		scope: "timeline",
		keys: "Mod+Wheel",
		action: "surface.zoom",
		fixed: true,
	},
	{
		id: "delete",
		scope: "timeline",
		keys: "Delete, Backspace",
		action: "deleteSelection",
	},
	{
		id: "ripple_delete",
		scope: "timeline",
		keys: "Shift+Delete",
		action: "deleteSelection",
	},
	{
		id: "duplicate",
		scope: "timeline",
		keys: "Mod+D",
		action: "duplicateSelection",
	},
	{ id: "copy", scope: "timeline", keys: "Mod+C", action: "surface.clipboard" },
	{ id: "cut", scope: "timeline", keys: "Mod+X", action: "surface.clipboard" },
	{
		id: "paste",
		scope: "timeline",
		keys: "Mod+V",
		action: "surface.clipboard",
	},
	{ id: "volume_up", scope: "timeline", keys: "Mod+.", action: "adjustVolume" },
	{
		id: "volume_down",
		scope: "timeline",
		keys: "Mod+,",
		action: "adjustVolume",
	},
	{
		id: "add_caption",
		scope: "timeline",
		keys: "C",
		classic: "Shift+C",
		action: "addCaption",
	},
	{
		id: "add_text",
		scope: "timeline",
		keys: "T",
		classic: "Shift+T",
		action: "addText",
	},
	{ id: "rename", scope: "timeline", keys: "F2", action: "renameItem" },
	{
		id: "toggle_enabled",
		scope: "timeline",
		keys: "V",
		classic: "Shift+E",
		action: "toggleEnabled",
	},
	{ id: "link", scope: "timeline", keys: "~", action: "toggleLink" },
	{
		id: "extract_audio",
		scope: "timeline",
		keys: "Mod+Shift+S",
		action: "extractAudio",
	},
	{ id: "toggle_magnet", scope: "timeline", keys: "P", action: "toggleMagnet" },
	{
		id: "copy_attributes",
		scope: "timeline",
		keys: "Mod+Shift+C",
		action: "copyAttributes",
	},
	{
		id: "paste_attributes",
		scope: "timeline",
		keys: "Mod+Shift+V",
		action: "pasteAttributes",
	},
	{
		id: "toggle_snap",
		scope: "timeline",
		keys: "N",
		action: "surface.toggle_snap",
	},
	{
		id: "keyframe_panel",
		scope: "global",
		keys: "Alt+K",
		action: "surface.keyframe_panel",
	},
	{
		id: "speed_curve",
		scope: "global",
		keys: "Shift+B",
		action: "surface.speed_curve",
	},
	{
		id: "scroll",
		scope: "timeline",
		keys: "Wheel, Alt+Wheel",
		action: "surface.scroll",
		fixed: true,
	},
	{
		id: "preview_fit",
		scope: "global",
		keys: "Alt+Shift+Z",
		action: "surface.preview_zoom",
	},
	{
		id: "preview_zoom_in",
		scope: "global",
		keys: "Alt+Shift+=",
		action: "surface.preview_zoom",
	},
	{
		id: "preview_zoom_out",
		scope: "global",
		keys: "Alt+Shift+-",
		action: "surface.preview_zoom",
	},
	{ id: "import", scope: "global", keys: "Mod+I", action: "surface.import" },
	{ id: "export", scope: "global", keys: "Mod+E", action: "surface.export" },
	{ id: "save", scope: "global", keys: "Mod+S", action: "surface.saved" },
	{ id: "palette", scope: "text", keys: "Mod+K", action: "surface.palette" },
	{ id: "find", scope: "text", keys: "Mod+F", action: "surface.find" },
	{
		id: "panel_library",
		scope: "global",
		keys: "Alt+1",
		action: "surface.toggle_panel",
	},
	{
		id: "panel_inspector",
		scope: "global",
		keys: "Alt+2",
		action: "surface.toggle_panel",
	},
	{
		id: "panel_history",
		scope: "global",
		keys: "Alt+3",
		action: "surface.toggle_panel",
	},
	{
		id: "select_all",
		scope: "timeline",
		keys: "Mod+A",
		action: "surface.select_all",
	},
	{
		id: "cancel",
		scope: "global",
		keys: "Esc",
		action: "surface.cancel",
		fixed: true,
	},
	{
		id: "next_landmark",
		scope: "global",
		keys: "F6",
		action: "surface.next_landmark",
	},
	{
		id: "prev_landmark",
		scope: "global",
		keys: "Shift+F6",
		action: "surface.next_landmark",
	},
	{
		id: "grid_keys",
		scope: "timeline",
		keys: "Timeline grid keys",
		action: "surface.grid_keys",
		fixed: true,
	},
	{
		id: "cheat_sheet",
		scope: "global",
		keys: "?",
		action: "surface.cheat_sheet",
	},
];

// The rows the person can rebind: every row that is not fixed.
export const BINDING_IDS: readonly string[] = SHORTCUTS.filter(
	(row) => row.fixed !== true,
).map((row) => row.id);

export function isBindingId(id: string): boolean {
	return BINDING_IDS.includes(id);
}

export function rowById(id: string): Shortcut | undefined {
	return SHORTCUTS.find((row) => row.id === id);
}

// The keys of a row in a preset, before any override of the person.
export function presetKeys(row: Shortcut, preset: PresetId): string[] {
	return (preset === "classic" ? (row.classic ?? row.keys) : row.keys).split(
		", ",
	);
}

export function aliasKeys(row: Shortcut): string[] {
	return row.aliases === undefined ? [] : row.aliases.split(", ");
}

// Keys that no row may take: Tab, Enter and Esc belong to focus, forms and
// closing; the rest are the browser's and the system's (SDD-012 section 18.7).
export const RESERVED_KEYS: readonly string[] = [
	"Tab",
	"Shift+Tab",
	"Enter",
	"Esc",
	"Mod+N",
	"Mod+T",
	"Mod+W",
	"Mod+Q",
	"Mod+R",
	"Mod+L",
	"Mod+P",
	"Mod+O",
	"Mod+Tab",
	"Mod+Shift+N",
	"Mod+Shift+T",
	"Mod+Shift+W",
	"Mod+Shift+R",
	"Mod+Shift+F",
	"Mod+Shift+Delete",
	"Alt+Left",
	"Alt+Right",
	"F5",
	"F11",
	"F12",
];

// Two scopes overlap when a key pressed in one could also fire a row of the
// other. Every handler of the editor listens at the window, the player's
// (play, pause, shuttle) as well as the timeline's, so one press reaches all of
// them: a key held by a row of any scope cannot be taken by another row. The
// scope still says where a row acts (rowFor), not where its key may repeat.
export function scopesOverlap(_a: Scope, _b: Scope): boolean {
	return true;
}

const KEY_NAMES: Readonly<Record<string, string>> = {
	" ": "Space",
	ArrowLeft: "Left",
	ArrowRight: "Right",
	ArrowUp: "Up",
	ArrowDown: "Down",
	Escape: "Esc",
};

// The physical key of a punctuation key, for a combo with Alt: Option produces
// another character on a Mac (SDD-012 section 18.9 item 7).
const CODE_NAMES: Readonly<Record<string, string>> = {
	Comma: ",",
	Period: ".",
	Equal: "=",
	Minus: "-",
	Slash: "/",
	Backslash: "\\",
	BracketLeft: "[",
	BracketRight: "]",
	Semicolon: ";",
	Quote: "'",
	Backquote: "`",
};

const MODIFIER_KEYS = new Set(["Control", "Meta", "Alt", "Shift", "AltGraph"]);

export type ComboEvent = Pick<
	KeyboardEvent,
	"key" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey"
> &
	Partial<Pick<KeyboardEvent, "code">>;

function physicalName(event: ComboEvent): string | null {
	const code = event.code;
	if (code === undefined) return null;
	const letter = /^Key([A-Z])$/.exec(code);
	if (letter?.[1] !== undefined) return letter[1];
	const digit = /^Digit([0-9])$/.exec(code);
	if (digit?.[1] !== undefined) return digit[1];
	return CODE_NAMES[code] ?? null;
}

// The canonical text of a key press, in the form of the table: `Mod`, `Alt`,
// `Shift` and the key. Shift is part of it for a letter or a named key only:
// for `+` or `?` the character already says it. With Alt the physical key
// names it, and Shift is then always named (`Alt+Shift+=`). Null for a
// modifier alone.
export function comboFromEvent(event: ComboEvent): string | null {
	if (MODIFIER_KEYS.has(event.key)) return null;
	const physical = event.altKey ? physicalName(event) : null;
	const name = physical ?? KEY_NAMES[event.key] ?? event.key;
	const single = name.length === 1;
	const letter = single && name.toLowerCase() !== name.toUpperCase();
	const shifted = event.shiftKey && (letter || !single || physical !== null);
	const parts: string[] = [];
	if (hasMod(event)) parts.push("Mod");
	if (event.altKey) parts.push("Alt");
	if (shifted) parts.push("Shift");
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
