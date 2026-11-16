import { useEffect } from "react";

import { useT } from "@/lib/i18n";
import { announce } from "../a11y/announce";
import {
	comboFromEvent,
	editShortcut,
	fixedRowFor,
	isSelectAllShortcut,
} from "../commands/keymap";
import { bindingFor } from "../commands/keymapStore";
import { useActions } from "../commands/useActions";
import { useRuntime } from "../state/EditorContext";
import {
	adjacentCut,
	adjacentMarker,
	idsOnSide,
	selectionSpan,
} from "./geometry";

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

// The items Mod+C and Mod+X took; Mod+V puts them at the playhead: a copy
// duplicates, a cut moves (the items stay until pasted, so no command is needed
// to bring back what a delete removed). UI state of the page: it is not project
// data and no other actor sees it.
let clipboard: { ids: readonly string[]; cut: boolean } = {
	ids: [],
	cut: false,
};

// The editing keys (SDD-012 sections 8.2, 15 and 18.7): split, trim to the
// playhead, Add caption and Add text have rebindable keys (Mod+B, Q, W, C and T
// by default, section 18.2); the fixed ones split every track, select a side of
// the playhead, copy, cut, paste and step the volume; Ctrl+Z undoes,
// Ctrl+Shift+Z and Ctrl+Y redo. Typing in a field never triggers them.
export function useShortcuts(openCaption: () => void): void {
	const actions = useActions();
	const runtime = useRuntime();
	const t = useT();
	useEffect(() => {
		// The fixed rows of section 18.7 that edit; true when the key was one.
		const fixedKey = (event: KeyboardEvent): boolean => {
			// The physical key: Option changes the character on a Mac.
			const combo =
				event.altKey && event.shiftKey && event.code === "KeyM"
					? "Alt+Shift+M"
					: comboFromEvent(event);
			const row = combo === null ? null : fixedRowFor(combo);
			if (row === null) return false;
			const { selection, focusedItem, playheadFrame } = runtime.ui.getState();
			const held =
				selection.items.length > 0
					? selection.items
					: focusedItem === null
						? []
						: [focusedItem];
			if (row === "split_all") void actions.splitAllAtPlayhead();
			else if (row === "marker") void actions.toggleMarkerAtPlayhead();
			else if (row === "next_marker") {
				const doc = runtime.documents.getState().document;
				if (doc === null) return false;
				const marker = adjacentMarker(
					(doc.markers ?? []).map((m) => m.frame),
					playheadFrame,
					combo === "Mod+Shift+Left" || combo === "Alt+Shift+M" ? -1 : 1,
				);
				if (marker !== null) runtime.uiWriter.setPlayhead(marker);
			} else if (row === "range_in_out") {
				if (combo === "I") runtime.uiWriter.setRangeIn(playheadFrame);
				else runtime.uiWriter.setRangeOut(playheadFrame);
			} else if (row === "range_select") {
				const doc = runtime.documents.getState().document;
				if (doc === null) return false;
				const span = selectionSpan(
					selection.items.flatMap((id) => {
						const item = doc.items[id];
						return item === undefined
							? []
							: [{ id, start: item.start, duration: item.duration }];
					}),
				);
				if (span === null) return false;
				runtime.uiWriter.setRangeIn(span.in);
				runtime.uiWriter.setRangeOut(span.out);
			} else if (row === "range_clear") runtime.uiWriter.clearRange();
			else if (row === "tool") {
				const blade = combo === "B";
				runtime.uiWriter.setTool(blade ? "blade" : "select");
				announce(
					blade
						? "ui.editor.announce.tool_blade"
						: "ui.editor.announce.tool_select",
				);
			} else if (row === "volume_up") void actions.adjustVolume(0.1);
			else if (row === "volume_down") void actions.adjustVolume(-0.1);
			else if (row === "select_side") {
				const doc = runtime.documents.getState().document;
				if (doc === null) return false;
				const items = Object.entries(doc.items).map(([id, item]) => ({
					id,
					start: item.start,
					duration: item.duration,
				}));
				runtime.uiWriter.select(
					idsOnSide(items, playheadFrame, combo === "[" ? "left" : "right"),
				);
			} else if (row === "copy" || row === "cut") {
				if (held.length === 0) return false;
				clipboard = { ids: [...held], cut: row === "cut" };
			} else if (row === "paste") {
				if (clipboard.ids.length === 0) return false;
				const { ids, cut } = clipboard;
				if (cut) clipboard = { ids: [], cut: false };
				void actions.pasteItems(ids, playheadFrame, cut);
			} else return false;
			event.preventDefault();
			return true;
		};
		const onKey = (event: KeyboardEvent): void => {
			if (isEditable(event.target)) return;
			if (event.altKey && !(event.shiftKey && event.code === "KeyM")) return;
			const edit = inOverlay(event.target) ? null : editShortcut(event);
			if (edit !== null) {
				event.preventDefault();
				if (edit === "delete") void actions.deleteSelection();
				else if (edit === "ripple_delete")
					void actions.deleteSelection("track");
				else if (edit === "duplicate") void actions.duplicateSelection();
				else if (edit === "toggle_enabled") void actions.toggleEnabled();
				else if (edit === "toggle_link") void actions.toggleLink();
				else if (edit === "extract_audio") void actions.extractAudio();
				else if (edit === "toggle_magnet") void actions.toggleMagnet();
				else {
					const { selection, focusedItem } = runtime.ui.getState();
					const only = selection.items.length === 1 ? selection.items[0] : null;
					const id = only ?? focusedItem;
					if (id !== null && id !== undefined) runtime.uiWriter.startRename(id);
				}
				return;
			}
			if (
				(event.key === "ArrowUp" || event.key === "ArrowDown") &&
				!event.shiftKey &&
				!(event.ctrlKey || event.metaKey) &&
				!inOverlay(event.target) &&
				!(
					event.target instanceof HTMLElement &&
					event.target.closest('[role="grid"]') !== null
				)
			) {
				const doc = runtime.documents.getState().document;
				if (doc === null) return;
				const cut = adjacentCut(
					Object.values(doc.items),
					runtime.ui.getState().playheadFrame,
					event.key === "ArrowDown" ? 1 : -1,
				);
				if (cut !== null) runtime.uiWriter.setPlayhead(cut);
				event.preventDefault();
				return;
			}
			if (isSelectAllShortcut(event) && !inOverlay(event.target)) {
				const doc = runtime.documents.getState().document;
				if (doc !== null) runtime.uiWriter.select(Object.keys(doc.items));
				event.preventDefault();
				return;
			}
			if (!inOverlay(event.target) && fixedKey(event)) return;
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
