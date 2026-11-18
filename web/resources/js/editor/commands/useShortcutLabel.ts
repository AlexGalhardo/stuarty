import { IS_MAC, labelWithKey } from "./keymap";
import { useBindings } from "./keymapStore";

// The tooltip text of a control: its label and the key of the row of the
// cheat sheet, "Split (Ctrl+B)" (SDD-012 section 18.2).
export function useShortcutLabel(): (label: string, rowId: string) => string {
	const bindings = useBindings();
	return (label, rowId) => labelWithKey(label, rowId, bindings, IS_MAC);
}
