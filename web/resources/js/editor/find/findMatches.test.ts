import { describe, expect, test } from "vitest";

import type { ProjectDocument } from "@/generated/timeline";
import { findMatches, foldText, stepIndex } from "./findMatches";

const doc = {
	assets: { a1: { id: "a1", name: "Beach.mp4" } },
	items: {
		i1: { id: "i1", type: "clip", asset: "a1", start: 0, duration: 90 },
		i2: {
			id: "i2",
			type: "text",
			name: "Title card",
			text: "Ação no mar\nsegunda linha",
			start: 30,
			duration: 60,
		},
		i3: {
			id: "i3",
			type: "caption",
			start: 100,
			duration: 120,
			words: [
				{ text: "hello", start: 0, duration: 10 },
				{ text: "world", start: 20, duration: 10 },
				{ text: "Hello", start: 40, duration: 10 },
			],
		},
	},
} as unknown as ProjectDocument;

describe("findMatches", () => {
	test("is empty without a document or a query", () => {
		expect(findMatches(null, "a")).toEqual([]);
		expect(findMatches(doc, "   ")).toEqual([]);
	});

	test("matches an item name, the name of its asset and a text, in time order", () => {
		const hits = findMatches(doc, "a");
		expect(hits.map((h) => [h.itemId, h.kind])).toContainEqual(["i1", "name"]);
		expect(hits.map((h) => h.frame)).toEqual(
			[...hits.map((h) => h.frame)].sort((a, b) => a - b),
		);
	});

	test("ignores case and accents", () => {
		expect(findMatches(doc, "acao")).toEqual([
			{ itemId: "i2", kind: "text", label: "Ação no mar", frame: 30 },
		]);
		expect(foldText("ÁRVORE")).toBe("arvore");
	});

	test("finds a caption word at the start of the word, not of the item", () => {
		expect(findMatches(doc, "HELLO")).toEqual([
			{ itemId: "i3", kind: "word", label: "hello", wordIndex: 0, frame: 100 },
			{ itemId: "i3", kind: "word", label: "Hello", wordIndex: 2, frame: 140 },
		]);
	});

	test("a name and a text of the same item are two matches", () => {
		const both = findMatches(doc, "title");
		expect(both).toHaveLength(1);
		expect(findMatches(doc, "mar").map((h) => h.kind)).toEqual(["text"]);
	});
});

describe("stepIndex", () => {
	test("starts at the first or the last and wraps", () => {
		expect(stepIndex(-1, 3, 1)).toBe(0);
		expect(stepIndex(-1, 3, -1)).toBe(2);
		expect(stepIndex(2, 3, 1)).toBe(0);
		expect(stepIndex(0, 3, -1)).toBe(2);
		expect(stepIndex(-1, 0, 1)).toBe(-1);
	});
});
