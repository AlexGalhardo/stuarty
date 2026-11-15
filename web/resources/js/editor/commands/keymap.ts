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
