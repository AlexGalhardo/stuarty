import { describe, expect, test } from "vitest";

import type { ImageHistoryEntry } from "../api/types";
import { entryText } from "./HistoryPanel";

const entry = (extra: Partial<ImageHistoryEntry>): ImageHistoryEntry => ({
	revision: 3,
	kind: "edit",
	actor: "local:ui",
	at: "2026-11-20T12:00:00Z",
	ops: ["layer.move"],
	changes: 1,
	mine: true,
	...extra,
});

describe("entryText", () => {
	test("a row says what the step did, in the words of the core, and not the names of its operations", () => {
		expect(
			entryText(entry({ description: "Moves a layer on the canvas." })),
		).toBe("Moves a layer on the canvas.");
		expect(
			entryText(
				entry({ ops: ["image.revert"], description: "Reverted to r1." }),
			),
		).toBe("Reverted to r1.");
		expect(
			entryText(
				entry({ kind: "undo", undo_of: 2, ops: [], description: "Undid r2." }),
			),
		).toBe("Undid r2.");
	});

	test("the panel composes no sentence: a row without a description shows the names of its operations", () => {
		expect(entryText(entry({}))).toBe("layer.move");
		expect(entryText(entry({ description: "" }))).toBe("layer.move");
	});
});
