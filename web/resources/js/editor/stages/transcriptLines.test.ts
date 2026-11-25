import { describe, expect, test } from "vitest";

import type { TranscriptSuggestion, TranscriptWord } from "../api/types";
import { MAX_LINE_WORDS, transcriptLines, wordKinds } from "./transcriptLines";

const word = (index: number, text: string): TranscriptWord => ({
	index,
	text,
	start: index * 10,
	end: index * 10 + 9,
	ranges: [],
});

describe("the lines of the transcript", () => {
	test("a line ends at the end of a sentence and starts at its first word", () => {
		const lines = transcriptLines([
			word(0, "Hello"),
			word(1, "world."),
			word(2, "Next"),
			word(3, "one?"),
			word(4, "tail"),
		]);
		expect(lines.map((l) => l.words.map((w) => w.text))).toEqual([
			["Hello", "world."],
			["Next", "one?"],
			["tail"],
		]);
		expect(lines.map((l) => l.start)).toEqual([0, 20, 40]);
	});

	test("a transcript with no punctuation is cut into lines of at most 14 words", () => {
		const words = Array.from({ length: MAX_LINE_WORDS + 3 }, (_, i) =>
			word(i, "a"),
		);
		const lines = transcriptLines(words);
		expect(lines).toHaveLength(2);
		expect(lines[0]?.words).toHaveLength(MAX_LINE_WORDS);
	});
});

describe("the highlight of the suggestions", () => {
	test("every word of a suggestion takes its kind", () => {
		const suggestions = [
			{ kind: "filler", from_word: 2, to_word: 2 },
			{ kind: "repetition", from_word: 5, to_word: 6 },
		] as TranscriptSuggestion[];
		const kinds = wordKinds(suggestions);
		expect(kinds.get(2)).toBe("filler");
		expect(kinds.get(5)).toBe("repetition");
		expect(kinds.get(6)).toBe("repetition");
		expect(kinds.has(3)).toBe(false);
	});
});
