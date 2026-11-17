import { useMemo } from "react";

import { useCanEdit } from "../commands/useActions";
import { useDocumentState, useUiState } from "../state/EditorContext";

export type ItemType = "clip" | "audio" | "text" | "caption" | "shape";

// The selected items a panel card may act on, in selection order. A card is
// disabled, with its reason in the `aria-describedby` paragraph, when none is.
export function useApplyTarget(types: readonly ItemType[]): {
	ids: readonly string[];
	canApply: boolean;
} {
	const canEdit = useCanEdit();
	const selection = useUiState((s) => s.selection.items);
	// A string, so the selector returns a value that compares equal.
	const joined = useDocumentState((s) =>
		selection
			.filter((id) => {
				const type = s.document?.items[id]?.type;
				return (
					type !== undefined && (types as readonly string[]).includes(type)
				);
			})
			.join(" "),
	);
	const ids = useMemo(() => (joined === "" ? [] : joined.split(" ")), [joined]);
	return { ids, canApply: canEdit && ids.length > 0 };
}
