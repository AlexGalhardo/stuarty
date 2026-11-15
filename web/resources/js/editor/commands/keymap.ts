// The keyboard map of SDD-012 section 18.2 that is shared by more than one
// component. The Prototype shortcuts stay in timeline/useShortcuts.ts; every
// key here calls an action of the registry or opens a surface of the editor.

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
