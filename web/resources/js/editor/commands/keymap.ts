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
