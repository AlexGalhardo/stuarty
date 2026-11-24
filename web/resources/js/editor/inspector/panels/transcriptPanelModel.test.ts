import { describe, expect, test } from "vitest";

import type { TranscriptSuggestion } from "../../api/types";
import {
	checkedRanges,
	extendSelection,
	initialChecked,
	isSelected,
	selectedRange,
	toggled,
} from "./transcriptPanelModel";

const suggestion = (
	from: number,
	to: number,
	strength: "strong" | "weak",
): TranscriptSuggestion => ({
	kind: "filler",
	strength,
	from_word: from,
	to_word: to,
	text: "um",
	start_ms: 0,
	end_ms: 100,
});

describe("the selection of words", () => {
	test("a click selects one word and a click with Shift extends from the first", () => {
		const one = extendSelection(null, 4, false);
		expect(selectedRange(one)).toEqual({ from: 4, to: 4 });
		const wide = extendSelection(one, 9, true);
		expect(selectedRange(wide)).toEqual({ from: 4, to: 9 });
		const backwards = extendSelection(wide, 2, true);
		expect(selectedRange(backwards)).toEqual({ from: 2, to: 4 });
		expect(isSelected(backwards, 3)).toBe(true);
		expect(isSelected(backwards, 5)).toBe(false);
	});

	test("Shift with nothing selected selects the word, a plain click starts again", () => {
		expect(selectedRange(extendSelection(null, 3, true))).toEqual({
			from: 3,
			to: 3,
		});
		const was = extendSelection(null, 1, false);
		expect(selectedRange(extendSelection(was, 7, false))).toEqual({
			from: 7,
			to: 7,
		});
		expect(selectedRange(null)).toBeNull();
		expect(isSelected(null, 0)).toBe(false);
	});
});

describe("the review before removal", () => {
	const list = [
		suggestion(5, 5, "weak"),
		suggestion(1, 1, "strong"),
		suggestion(8, 9, "strong"),
	];

	test("strong suggestions start checked and weak ones unchecked", () => {
		expect([...initialChecked(list)]).toEqual([1, 2]);
	});

	test("the ranges sent are the checked ones, in the order of the transcript", () => {
		expect(checkedRanges(list, new Set([2, 0, 1]))).toEqual([
			{ from: 1, to: 1 },
			{ from: 5, to: 5 },
			{ from: 8, to: 9 },
		]);
		expect(checkedRanges(list, new Set())).toEqual([]);
	});

	test("toggling checks and unchecks without changing the old set", () => {
		const first = new Set([1]);
		const on = toggled(first, 0);
		expect([...on].sort()).toEqual([0, 1]);
		expect([...first]).toEqual([1]);
		expect([...toggled(on, 1)]).toEqual([0]);
	});
});
