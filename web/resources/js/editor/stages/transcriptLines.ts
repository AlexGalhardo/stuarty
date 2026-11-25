import type { TranscriptSuggestion, TranscriptWord } from "../api/types";

// How the transcript is laid out on the edit by text screen (final design 26): a line for each sentence, with the
// time of its first word at the left. Only a layout: the words, their frames and the suggestions are the service's.

export interface TranscriptLine {
	// The frame of the asset where the line starts.
	start: number;
	words: readonly TranscriptWord[];
}

// The longest line, in words, for a transcript that has no punctuation (a model that writes none).
export const MAX_LINE_WORDS = 14;

const ENDS_SENTENCE = /[.!?…。！？]["')\]]*$/u;

export function transcriptLines(
	words: readonly TranscriptWord[],
): TranscriptLine[] {
	const lines: TranscriptLine[] = [];
	let current: TranscriptWord[] = [];
	const close = (): void => {
		const first = current[0];
		if (first !== undefined) lines.push({ start: first.start, words: current });
		current = [];
	};
	for (const word of words) {
		current.push(word);
		if (ENDS_SENTENCE.test(word.text) || current.length >= MAX_LINE_WORDS) {
			close();
		}
	}
	close();
	return lines;
}

// The kind of suggestion each word of the transcript belongs to, for the highlight: a filler word or a repetition.
export function wordKinds(
	suggestions: readonly TranscriptSuggestion[],
): Map<number, TranscriptSuggestion["kind"]> {
	const kinds = new Map<number, TranscriptSuggestion["kind"]>();
	for (const suggestion of suggestions) {
		for (let i = suggestion.from_word; i <= suggestion.to_word; i++) {
			kinds.set(i, suggestion.kind);
		}
	}
	return kinds;
}
