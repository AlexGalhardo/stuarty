import { describe, expect, it } from "vitest";
import type { DocumentState } from "./documentStore";
import { selectOrderedTrackIds, selectTrackItemIds } from "./selectors";

// useSyncExternalStore loops forever (React error 185) when a selector hands
// back a new array for the same state, which the editor hit before the first
// document arrived.
describe("selectors keep a stable result", () => {
	const empty = { document: null } as unknown as DocumentState;

	it("returns the same list for an empty state", () => {
		expect(selectOrderedTrackIds(empty)).toBe(selectOrderedTrackIds(empty));
		expect(selectTrackItemIds(empty, "trk_x")).toBe(
			selectTrackItemIds(empty, "trk_x"),
		);
	});
});
