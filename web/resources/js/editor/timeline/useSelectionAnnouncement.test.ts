import { describe, expect, test } from "vitest";

import { droppedByTheDocument } from "./useSelectionAnnouncement";

describe("droppedByTheDocument", () => {
	const gone = new Set(["a"]);
	const exists = (id: string): boolean => !gone.has(id);

	test("a selected item deleted from the document is the document's change, not the person's", () => {
		expect(droppedByTheDocument(["a"], [], exists)).toBe(true);
		expect(droppedByTheDocument(["a", "b"], ["b"], exists)).toBe(true);
	});

	test("a person who clears or changes the selection is told", () => {
		expect(droppedByTheDocument(["b"], [], exists)).toBe(false);
		expect(droppedByTheDocument(["a"], ["b"], exists)).toBe(false);
		expect(droppedByTheDocument([], ["b"], exists)).toBe(false);
		expect(droppedByTheDocument(["b"], ["b"], exists)).toBe(false);
	});
});
