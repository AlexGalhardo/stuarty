import { useEffect } from "react";

import { useT } from "@/lib/i18n";
import { announce } from "../a11y/announce";
import {
	pasteFromClipboard,
	selectAllItems,
	takeToClipboard,
} from "../commands/clipboard";
import { rowFor } from "../commands/keymapStore";
import { inOverlay, isEditable } from "../commands/keyTarget";
import { useActions } from "../commands/useActions";
import { useRuntime } from "../state/EditorContext";
import {
	adjacentCut,
	adjacentMarker,
	idsOnSide,
	selectionSpan,
} from "./geometry";

// The rows an open dialog or menu does not take away from the editor: a person
// who is in the export dialog can still undo.
const ALLOWED_IN_OVERLAY: ReadonlySet<string> = new Set(["undo", "redo"]);

// The editing keys (SDD-012 sections 8.2, 15, 18.7 and 18.9): every key is a row
// of the registry (commands/keymap.ts) and is read through rowFor(), so a
// rebinding is live at once and no key is written here. Typing in a field never
// triggers them, and a dialog or menu keeps its own keys.
export function useShortcuts(openCaption: () => void): void {
	const actions = useActions();
	const runtime = useRuntime();
	const t = useT();
	useEffect(() => {
		const onKey = (event: KeyboardEvent): void => {
			if (isEditable(event.target)) return;
			const row = rowFor(event, ["global", "timeline"]);
			if (row === null) return;
			if (inOverlay(event.target) && !ALLOWED_IN_OVERLAY.has(row)) return;
			const { selection, focusedItem, playheadFrame } = runtime.ui.getState();
			const document_ = runtime.documents.getState().document;
			// The items a slide, roll or slip acts on: the selection, or else the
			// one that has the focus.
			const acted =
				selection.items.length > 0
					? selection.items
					: focusedItem === null || focusedItem === undefined
						? []
						: [focusedItem];
			switch (row) {
				case "undo":
					void actions.undo();
					break;
				case "redo":
					void actions.redo();
					break;
				case "split":
					void actions.splitAtPlayhead();
					break;
				case "split_all":
					void actions.splitAllAtPlayhead();
					break;
				case "trim_start":
					void actions.trimToPlayhead("start");
					break;
				case "trim_end":
					void actions.trimToPlayhead("end");
					break;
				case "add_caption":
					openCaption();
					break;
				case "add_text":
					void actions.addText(t("ui.editor.defaults.text"));
					break;
				case "marker":
					void actions.toggleMarkerAtPlayhead();
					break;
				case "prev_marker":
				case "next_marker": {
					if (document_ === null) return;
					const marker = adjacentMarker(
						(document_.markers ?? []).map((m) => m.frame),
						playheadFrame,
						row === "prev_marker" ? -1 : 1,
					);
					if (marker !== null) runtime.uiWriter.setPlayhead(marker);
					break;
				}
				case "range_in":
					runtime.uiWriter.setRangeIn(playheadFrame);
					break;
				case "range_out":
					runtime.uiWriter.setRangeOut(playheadFrame);
					break;
				case "range_select": {
					if (document_ === null) return;
					const span = selectionSpan(
						selection.items.flatMap((id) => {
							const item = document_.items[id];
							return item === undefined
								? []
								: [{ id, start: item.start, duration: item.duration }];
						}),
					);
					if (span === null) return;
					runtime.uiWriter.setRangeIn(span.in);
					runtime.uiWriter.setRangeOut(span.out);
					break;
				}
				case "range_clear":
					runtime.uiWriter.clearRange();
					break;
				case "tool_blade":
				case "tool_select":
					runtime.uiWriter.setTool(row === "tool_blade" ? "blade" : "select");
					announce(
						row === "tool_blade"
							? "ui.editor.announce.tool_blade"
							: "ui.editor.announce.tool_select",
					);
					break;
				case "volume_up":
					void actions.adjustVolume(0.1);
					break;
				case "volume_down":
					void actions.adjustVolume(-0.1);
					break;
				case "select_left":
				case "select_right": {
					if (document_ === null) return;
					const items = Object.entries(document_.items).map(([id, item]) => ({
						id,
						start: item.start,
						duration: item.duration,
					}));
					runtime.uiWriter.select(
						idsOnSide(
							items,
							playheadFrame,
							row === "select_left" ? "left" : "right",
						),
					);
					break;
				}
				case "copy":
				case "cut":
					if (!takeToClipboard(runtime, row === "cut")) return;
					break;
				case "paste":
					if (!pasteFromClipboard(runtime, actions)) return;
					break;
				case "delete":
					void actions.deleteSelection();
					break;
				case "ripple_delete":
					void actions.deleteSelection("track");
					break;
				case "duplicate":
					void actions.duplicateSelection();
					break;
				case "toggle_enabled":
					void actions.toggleEnabled();
					break;
				case "link":
					void actions.toggleLink();
					break;
				case "extract_audio":
					void actions.extractAudio();
					break;
				case "toggle_magnet":
					void actions.toggleMagnet();
					break;
				case "copy_attributes":
					actions.copyAttributes();
					break;
				case "paste_attributes":
					void actions.pasteAttributes();
					break;
				case "rename": {
					const only = selection.items.length === 1 ? selection.items[0] : null;
					const id = only ?? focusedItem;
					if (id !== null && id !== undefined) runtime.uiWriter.startRename(id);
					break;
				}
				case "slide_back":
				case "slide_forward":
				case "roll_back":
				case "roll_forward":
				case "slip_back":
				case "slip_forward": {
					const sign = row.endsWith("_back") ? -1 : 1;
					const run = row.startsWith("slide")
						? actions.slideItems
						: row.startsWith("roll")
							? actions.rollItems
							: actions.slipItems;
					void run(acted, sign);
					break;
				}
				case "prev_cut":
				case "next_cut": {
					// On the grid the arrows move the focus (SDD-012 section 7.10).
					if (
						event.target instanceof HTMLElement &&
						event.target.closest('[role="grid"]') !== null
					)
						return;
					if (document_ === null) return;
					const cut = adjacentCut(
						Object.values(document_.items),
						playheadFrame,
						row === "next_cut" ? 1 : -1,
					);
					if (cut !== null) runtime.uiWriter.setPlayhead(cut);
					break;
				}
				case "select_all":
					selectAllItems(runtime);
					break;
				default:
					return;
			}
			event.preventDefault();
		};
		window.addEventListener("keydown", onKey);
		return () => window.removeEventListener("keydown", onKey);
	}, [actions, openCaption, runtime, t]);
}
