import { createStore, readOnly, useStore } from "./createStore";
import { readShortcutsSeen, writeShortcutsSeen } from "./prefs";

// Whether the first-run tips are on screen (SDD-012 section 18.5). They start
// visible when this browser has not seen them; "Show tips again" shows them
// whatever was remembered.
const store = createStore<{ visible: boolean }>({
	visible: !readShortcutsSeen(),
});
export const tipsStore = readOnly(store);

export function useTipsVisible(): boolean {
	return useStore(tipsStore, (s) => s.visible);
}

// Showing the tips is what remembers them: a reload does not show them again.
export function rememberTips(): void {
	writeShortcutsSeen();
}

export function dismissTips(): void {
	store.setState(() => ({ visible: false }));
}

export function showTipsAgain(): void {
	store.setState(() => ({ visible: true }));
}
