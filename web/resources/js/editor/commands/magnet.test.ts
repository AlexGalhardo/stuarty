import { describe, expect, test } from "vitest";

import type { DocumentState } from "../state/documentStore";
import { magnetTrackId } from "./magnet";

// Two visual tracks and an audio one; the order string decides the main track.
const state = {
	document: {
		tracks: {
			trk_audio: { id: "trk_audio", kind: "audio", order: "a" },
			trk_main: { id: "trk_main", kind: "visual", order: "b" },
			trk_top: { id: "trk_top", kind: "visual", order: "c" },
		},
		items: {
			itm_top: { id: "itm_top", track: "trk_top" },
		},
	},
} as unknown as DocumentState;

const none = { selection: { items: [], track: null }, focusedItem: null };

describe("the track the magnet acts on", () => {
	test("a selected track wins", () => {
		expect(
			magnetTrackId(state, {
				selection: { items: ["itm_top"], track: "trk_audio" },
				focusedItem: null,
			} as never),
		).toBe("trk_audio");
	});

	test("else the track of the selected item, then of the focused one", () => {
		expect(
			magnetTrackId(state, {
				selection: { items: ["itm_top"], track: null },
				focusedItem: null,
			} as never),
		).toBe("trk_top");
		expect(
			magnetTrackId(state, { ...none, focusedItem: "itm_top" } as never),
		).toBe("trk_top");
	});

	test("else the first visual track, and none without a document", () => {
		expect(magnetTrackId(state, none as never)).toBe("trk_main");
		expect(
			magnetTrackId(
				{ document: null } as unknown as DocumentState,
				none as never,
			),
		).toBeNull();
	});
});
