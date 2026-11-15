import { useMemo } from "react";

import {
	useDocumentState,
	useRuntime,
	useUiState,
} from "../state/EditorContext";
import { type Actions, createActions } from "./actions";
import { containsStrictly } from "./builders";

export function useActions(): Actions {
	const runtime = useRuntime();
	return useMemo(() => createActions(runtime), [runtime]);
}

// Whether a selected item contains the playhead strictly: the affordance of
// split and trim (SDD-012 section 8.3), never a prediction of success.
export function useCanCut(): boolean {
	const playhead = useUiState((s) => s.playheadFrame);
	const selection = useUiState((s) => s.selection.items);
	return useDocumentState((s) =>
		selection.some((id) => {
			const item = s.document?.items[id];
			return (
				item !== undefined &&
				containsStrictly(
					{ id, start: item.start, duration: item.duration },
					playhead,
				)
			);
		}),
	);
}
