import type { ProjectDocument } from "@/generated/timeline";

import { wordFrame } from "../commands/units";
import type { DeepReadonly } from "../state/applyDiff";

// The find bar of VV-052 (SDD-012 section 18.8): which items of the open
// project contain a text. It reads what the client already has and only
// chooses what to show: it changes nothing and computes no edit result.

export type MatchKind = "name" | "text" | "word";

export interface FindMatch {
	itemId: string;
	kind: MatchKind;
	// The text that matched, as stored, for the list of results.
	label: string;
	// The caption word that matched; present for `kind === "word"`.
	wordIndex?: number;
	// Where the playhead goes: the start of the item, or of the word.
	frame: number;
}

// Letters compared without case and without accents, so "acao" finds "Ação".
export function foldText(value: string): string {
	return value.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase();
}

export function findMatches(
	document: DeepReadonly<ProjectDocument> | null,
	query: string,
): FindMatch[] {
	const needle = foldText(query.trim());
	if (document === null || needle === "") return [];
	const found: FindMatch[] = [];
	for (const item of Object.values(document.items)) {
		const names = [item.name, document.assets[item.asset ?? ""]?.name];
		for (const name of names) {
			if (name !== undefined && foldText(name).includes(needle)) {
				found.push({
					itemId: item.id,
					kind: "name",
					label: name,
					frame: item.start,
				});
				break;
			}
		}
		if (item.text !== undefined && foldText(item.text).includes(needle)) {
			found.push({
				itemId: item.id,
				kind: "text",
				label: item.text.split("\n")[0] ?? item.text,
				frame: item.start,
			});
		}
		item.words?.forEach((word, wordIndex) => {
			if (!foldText(word.text).includes(needle)) return;
			found.push({
				itemId: item.id,
				kind: "word",
				label: word.text,
				wordIndex,
				frame: wordFrame(item.start, word.start),
			});
		});
	}
	return found.sort(
		(a, b) =>
			a.frame - b.frame ||
			a.itemId.localeCompare(b.itemId) ||
			(a.wordIndex ?? -1) - (b.wordIndex ?? -1),
	);
}

// The match after `current` (or before, with direction -1), wrapping around;
// -1 means none is current yet, so the first Enter goes to the first match and
// the first Shift+Enter to the last.
export function stepIndex(
	current: number,
	count: number,
	direction: 1 | -1,
): number {
	if (count === 0) return -1;
	if (current < 0) return direction === 1 ? 0 : count - 1;
	return (current + direction + count) % count;
}
