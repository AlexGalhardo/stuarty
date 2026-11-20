import { useEffect, useRef } from "react";

import { useT } from "@/lib/i18n";
import { announce } from "../a11y/announce";
import { displayEnd, displayStart } from "../commands/units";
import { useDocumentState, useUiState } from "../state/EditorContext";
import { readTimeDisplay } from "../state/prefs";
import { selectFps, selectOrderedTrackIds } from "../state/selectors";
import { trackLabel } from "../ui/trackLabel";
import { DEFAULT_FPS, type Fps } from "./geometry";

// The text alternative of the selection (SDD-012 section 18.10): when the selection changes the polite
// region says what is selected, for one item with its type, name, times and track, for several with a count.
// It reads the read model and never edits it.
export function useSelectionAnnouncement(): void {
	const t = useT();
	const selection = useUiState((s) => s.selection.items);
	const items = useDocumentState((s) => s.document?.items);
	const tracks = useDocumentState((s) => s.document?.tracks);
	const assets = useDocumentState((s) => s.document?.assets);
	const order = useDocumentState(selectOrderedTrackIds);
	const fps: Fps = useDocumentState(selectFps) ?? DEFAULT_FPS;
	const latest = useRef({ items, tracks, assets, order, fps, t });
	latest.current = { items, tracks, assets, order, fps, t };
	const key = selection.join(",");
	const first = useRef(true);
	const previous = useRef<readonly string[]>(selection);

	// biome-ignore lint/correctness/useExhaustiveDependencies: only a change of the selection speaks; the document is read through the ref
	useEffect(() => {
		const before = previous.current;
		previous.current = selection;
		if (first.current) {
			first.current = false;
			return;
		}
		const now = latest.current;
		// The read model removed selected items (a delete of the person or of an agent): the edit has its own
		// sentence in the same one-slot region ("Deleted, 1 item", "claude-code changed the project"), which a
		// "Selection cleared" right after would replace.
		if (
			droppedByTheDocument(
				before,
				selection,
				(id) => now.items?.[id] !== undefined,
			)
		) {
			return;
		}
		if (selection.length === 0) {
			announce("ui.editor.announce.selection_cleared");
			return;
		}
		const only =
			selection.length === 1 ? now.items?.[selection[0] ?? ""] : null;
		if (only === null || only === undefined) {
			announce("ui.editor.announce.selected_many", { count: selection.length });
			return;
		}
		const track = now.tracks?.[only.track];
		const kindOf = (id: string): "audio" | "visual" =>
			now.tracks?.[id]?.kind === "audio" ? "audio" : "visual";
		const type = now.t(`ui.editor.item.${only.type}`);
		const asset =
			only.asset === undefined ? undefined : now.assets?.[only.asset];
		const mode = readTimeDisplay();
		announce("ui.editor.announce.selected_one", {
			name: only.name ?? asset?.name ?? only.text?.split("\n")[0] ?? type,
			type,
			start: displayStart(only, now.fps, mode),
			end: displayEnd(only, now.fps, mode),
			track:
				track?.name === undefined || track.name === ""
					? trackLabel(only.track, now.order, kindOf)
					: track.name,
		});
	}, [key]);
}

// True when the selection only lost items, and every item it lost is gone from the document: the change was
// made by the read model, not by the person.
export function droppedByTheDocument(
	before: readonly string[],
	after: readonly string[],
	exists: (id: string) => boolean,
): boolean {
	if (after.some((id) => !before.includes(id))) return false;
	const lost = before.filter((id) => !after.includes(id));
	return lost.length > 0 && lost.every((id) => !exists(id));
}
