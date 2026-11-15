import { useEffect } from "react";

import { useT } from "@/lib/i18n";
import { editShortcut, isSelectAllShortcut } from "../commands/keymap";
import { bindingFor } from "../commands/keymapStore";
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

const EDIT_BINDINGS = [
	"split",
	"trim_start",
	"trim_end",
	"add_caption",
	"add_text",
] as const;

// The keys of the Prototype (SDD-012 sections 8.2 and 15): split, trim to the
// playhead, Add caption and Add text have rebindable keys (S, [, ], C and T by
// default, section 18.2); Ctrl+Z undoes, Ctrl+Shift+Z and Ctrl+Y redo. Typing
// in a field never triggers them.
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
			if (isSelectAllShortcut(event) && !inOverlay(event.target)) {
				const doc = runtime.documents.getState().document;
				if (doc !== null) runtime.uiWriter.select(Object.keys(doc.items));
				event.preventDefault();
				return;
			}
			const bound = inOverlay(event.target)
				? null
				: bindingFor(event, EDIT_BINDINGS);
			if (bound !== null) {
				if (bound === "split") void actions.splitAtPlayhead();
				else if (bound === "trim_start") void actions.trimToPlayhead("start");
				else if (bound === "trim_end") void actions.trimToPlayhead("end");
				else if (bound === "add_caption") openCaption();
				else void actions.addText(t("ui.editor.defaults.text"));
				event.preventDefault();
				return;
			}
			if (!(event.ctrlKey || event.metaKey)) return;
			const key = event.key.toLowerCase();
			if (key === "z" && !event.shiftKey) void actions.undo();
			else if ((key === "z" && event.shiftKey) || key === "y") {
				void actions.redo();
			} else return;
			event.preventDefault();
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, [actions, openCaption, runtime, t]);
}
