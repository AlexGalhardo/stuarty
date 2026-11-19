import { IS_MAC } from "./keymap";
import { labelWithKey, useKeymap } from "./keymapStore";

// The tooltip text of a control: its label and the key of the row of the
// cheat sheet, "Split (Ctrl+B)" (SDD-012 section 18.2).
export function useShortcutLabel(): (label: string, rowId: string) => string {
	const bindings = useKeymap();
	return (label, rowId) => labelWithKey(label, rowId, bindings, IS_MAC);
}
