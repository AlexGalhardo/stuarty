import { useEffect } from "react";

import { useT } from "@/lib/i18n";
import { editShortcut } from "../commands/keymap";
import { useActions } from "../commands/useActions";
import { useRuntime } from "../state/EditorContext";

function isEditable(target: EventTarget | null): boolean {
	if (!(target instanceof HTMLElement)) return false;
	return (
		target.isContentEditable ||
		["INPUT", "TEXTAREA", "SELECT"].includes(target.tagName)
	);
}

// A key aimed at an open dialog or menu is not for the timeline behind it.
function inOverlay(target: EventTarget | null): boolean {
	return (
		target instanceof HTMLElement &&
		target.closest('[role="dialog"], [role="alertdialog"], [role="menu"]') !==
			null
	);
}

// The keys of the Prototype (SDD-012 sections 8.2 and 15): S splits, [ and ]
// trim to the playhead, C opens Add caption, T adds a text item, Ctrl+Z undoes, Ctrl+Shift+Z and
// Ctrl+Y redo. Typing in a field never triggers them.
export function useShortcuts(openCaption: () => void): void {
	const actions = useActions();
	const runtime = useRuntime();
	const t = useT();
	useEffect(() => {
		const onKey = (event: KeyboardEvent): void => {
			if (isEditable(event.target) || event.altKey) return;
			const edit = inOverlay(event.target) ? null : editShortcut(event);
			if (edit !== null) {
				event.preventDefault();
				if (edit === "delete") void actions.deleteSelection();
				else if (edit === "ripple_delete")
					void actions.deleteSelection("track");
				else if (edit === "duplicate") void actions.duplicateSelection();
				else {
					const { selection, focusedItem } = runtime.ui.getState();
					const only = selection.items.length === 1 ? selection.items[0] : null;
					const id = only ?? focusedItem;
					if (id !== null && id !== undefined) runtime.uiWriter.startRename(id);
				}
				return;
			}
			const mod = event.ctrlKey || event.metaKey;
			const key = event.key.toLowerCase();
			if (mod) {
				if (key === "z" && !event.shiftKey) void actions.undo();
				else if ((key === "z" && event.shiftKey) || key === "y") {
					void actions.redo();
				} else return;
				event.preventDefault();
				return;
			}
			if (key === "s") void actions.splitAtPlayhead();
			else if (key === "[") void actions.trimToPlayhead("start");
			else if (key === "]") void actions.trimToPlayhead("end");
			else if (key === "c") openCaption();
			else if (key === "t") void actions.addText(t("ui.editor.defaults.text"));
			else return;
			event.preventDefault();
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, [actions, openCaption, runtime, t]);
}
