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

	// biome-ignore lint/correctness/useExhaustiveDependencies: only a change of the selection speaks; the document is read through the ref
	useEffect(() => {
		if (first.current) {
			first.current = false;
			return;
		}
		const now = latest.current;
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
