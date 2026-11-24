import type { Hunk } from "./client";

// The two sides of a comparison, as the core's hunks fall on them: the revision compared from holds the paragraphs
// that stayed and the ones taken out, the revision compared to holds the paragraphs that stayed and the ones put
// in. The hunks are the core's, in the core's order; this only says on which side each one is drawn.
export interface Sides {
	from: Hunk[];
	to: Hunk[];
}

export function sidesOf(hunks: readonly Hunk[]): Sides {
	return {
		from: hunks.filter((hunk) => hunk.op !== "added"),
		to: hunks.filter((hunk) => hunk.op !== "removed"),
	};
}

// The paragraphs the core marked as changed (those that are not equal), to tell whether anything differs.
export function changedCount(hunks: readonly Hunk[]): number {
	return hunks.filter((hunk) => hunk.op !== "equal").length;
}
