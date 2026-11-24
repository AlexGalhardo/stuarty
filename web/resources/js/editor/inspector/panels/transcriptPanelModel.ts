import type {
	EditResult,
	JsonValue,
	TranscriptSuggestion,
} from "../../api/types";

// What the transcript panel keeps between gestures (SDD-023 section 6). Nothing here finds a frame, a length or a
// match between words: it names words by index and the core does the rest.

export interface WordSelection {
	anchor: number;
	focus: number;
}

// A click selects one word, a click with Shift extends the selection from the word that was clicked first.
export function extendSelection(
	current: WordSelection | null,
	index: number,
	extend: boolean,
): WordSelection {
	if (!extend || current === null) return { anchor: index, focus: index };
	return { anchor: current.anchor, focus: index };
}

export function selectedRange(
	selection: WordSelection | null,
): { from: number; to: number } | null {
	if (selection === null) return null;
	return {
		from: Math.min(selection.anchor, selection.focus),
		to: Math.max(selection.anchor, selection.focus),
	};
}

export function isSelected(
	selection: WordSelection | null,
	index: number,
): boolean {
	const range = selectedRange(selection);
	return range !== null && index >= range.from && index <= range.to;
}

// The review before removal: strong suggestions start checked, weak ones start unchecked (SDD-023 section 6).
export function initialChecked(
	suggestions: readonly TranscriptSuggestion[],
): ReadonlySet<number> {
	const checked = new Set<number>();
	suggestions.forEach((suggestion, i) => {
		if (suggestion.strength === "strong") checked.add(i);
	});
	return checked;
}

export function toggled(
	checked: ReadonlySet<number>,
	index: number,
): ReadonlySet<number> {
	const next = new Set(checked);
	if (next.has(index)) next.delete(index);
	else next.add(index);
	return next;
}

// The ranges of the checked suggestions, in the order of the transcript.
export function checkedRanges(
	suggestions: readonly TranscriptSuggestion[],
	checked: ReadonlySet<number>,
): { from: number; to: number }[] {
	return suggestions
		.flatMap((suggestion, i) =>
			checked.has(i)
				? [{ from: suggestion.from_word, to: suggestion.to_word }]
				: [],
		)
		.sort((a, b) => a.from - b.from);
}

// What the service says about the edges of the cuts it made (`effective.snap` of transcript.delete_range, SDD-002 C.87):
// how many edges moved into a silence, were already in one, or found none. `missing` is a cut made with no silence
// analysis of the asset. The panel adds up what it was told and finds no frame.
export interface SnapSummary {
	missing: boolean;
	moved: number;
	inside: number;
	none: number;
}

function isRecord(value: JsonValue | undefined): value is {
	[key: string]: JsonValue;
} {
	return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function snapSummary(result: EditResult): SnapSummary | null {
	const sum: SnapSummary = { missing: false, moved: 0, inside: 0, none: 0 };
	let seen = false;
	for (const entry of result.effective) {
		if (entry.op !== "transcript.delete_range") continue;
		const snap = entry.effective.snap;
		if (!isRecord(snap)) continue;
		seen = true;
		if (snap.analysis === "missing") sum.missing = true;
		for (const edge of [snap.start, snap.end]) {
			if (!isRecord(edge)) continue;
			if (edge.result === "moved") sum.moved++;
			else if (edge.result === "inside") sum.inside++;
			else sum.none++;
		}
	}
	return seen ? sum : null;
}
