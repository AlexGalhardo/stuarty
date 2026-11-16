import type { DocumentState } from "../state/documentStore";
import { selectOrderedTrackIds, selectTrack } from "../state/selectors";
import type { UiState } from "../state/uiStore";

// The track the P key and the Magnet button act on: the selected track, else
// the track of the first selected item (or of the item that holds the keyboard
// focus), else the first visual track, the main one. Which track is a choice of
// the screen; whether it is packed is the core's.
export function magnetTrackId(
	state: DocumentState,
	ui: Pick<UiState, "selection" | "focusedItem">,
): string | null {
	const doc = state.document;
	if (doc === null) return null;
	if (ui.selection.track !== null && doc.tracks[ui.selection.track]) {
		return ui.selection.track;
	}
	const acted =
		ui.selection.items.length > 0
			? ui.selection.items
			: ui.focusedItem === null
				? []
				: [ui.focusedItem];
	for (const id of acted) {
		const track = doc.items[id]?.track;
		if (track !== undefined) return track;
	}
	return (
		selectOrderedTrackIds(state).find(
			(id) => selectTrack(state, id)?.kind === "visual",
		) ?? null
	);
}
