import { hasMod } from "../commands/keymap";

// Mod+F opens the find bar (SDD-012 section 18.8).
export function isFindShortcut(
	event: Pick<
		KeyboardEvent,
		"key" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey"
	>,
): boolean {
	return (
		hasMod(event) &&
		!event.altKey &&
		!event.shiftKey &&
		event.key.toLowerCase() === "f"
	);
}
