import type { Item } from "@/generated/timeline";

import type { DeepReadonly } from "../state/applyDiff";
import type { DocumentState } from "../state/documentStore";
import { selectTrackItemIds } from "../state/selectors";
import { sortByStart } from "./geometry";

const NO_ITEMS: readonly DeepReadonly<Item>[] = [];

const orderedItemsCache = new Map<string, readonly DeepReadonly<Item>[]>();

// The items of a track in display order. The same array comes back while the
// items are the same objects, so a row subscribed to it renders only when one
// of its own items changed, not at every revision of the project.
export function selectTrackItems(
	state: DocumentState,
	trackId: string,
): readonly DeepReadonly<Item>[] {
	const items = state.document?.items;
	if (items === undefined) return NO_ITEMS;
	const next = sortByStart(
		selectTrackItemIds(state, trackId).map((id) => items[id]),
	);
	const previous = orderedItemsCache.get(trackId);
	if (
		previous !== undefined &&
		previous.length === next.length &&
		previous.every((item, i) => item === next[i])
	) {
		return previous;
	}
	orderedItemsCache.set(trackId, next);
	return next;
}
