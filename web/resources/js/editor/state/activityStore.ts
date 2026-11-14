import type { Change } from "../api/types";
import { createStore, type ReadStore, readOnly } from "./createStore";

// SDD-012 section 5.8: who edited last and what the remote edit touched.
export interface ActivityEntry {
	actor: string;
	revision: number;
	at: number;
	// Counts by object kind of the diff entries; the indicator turns them into
	// a localized sentence. The browser decides nothing from them.
	counts: { items: number; tracks: number; assets: number; other: number };
}

export interface ActivityState {
	entries: readonly ActivityEntry[];
	// actor -> time of its latest remote batch
	active: Readonly<Record<string, number>>;
	// ids of items named in a remote diff, outlined for HIGHLIGHT_MS
	highlighted: readonly string[];
}

export const HIGHLIGHT_MS = 1500;
export const ACTIVE_MS = 3000;
export const MAX_ENTRIES = 20;

export interface ActivityWriter {
	record(entry: ActivityEntry, itemIds: readonly string[]): void;
	clearHighlights(): void;
}

export function summarize(
	diff: readonly Change[] | undefined,
): ActivityEntry["counts"] {
	const counts = { items: 0, tracks: 0, assets: 0, other: 0 };
	for (const change of diff ?? []) {
		if (change.kind.startsWith("item.")) counts.items++;
		else if (change.kind.startsWith("track.")) counts.tracks++;
		else if (change.kind.startsWith("asset.")) counts.assets++;
		else counts.other++;
	}
	return counts;
}

export function changedItemIds(diff: readonly Change[] | undefined): string[] {
	return (diff ?? [])
		.filter((c) => c.kind.startsWith("item."))
		.map((c) => c.id);
}

// `local:claude-code` is shown as `claude-code`, `key:key_ab12cd34ef` as
// `key ab12`; anything else stays as it is (SDD-012 section 5.8 item 6).
export function actorLabel(actor: string): string {
	if (actor.startsWith("local:")) return actor.slice("local:".length);
	if (actor.startsWith("key:key_"))
		return `key ${actor.slice("key:key_".length, "key:key_".length + 4)}`;
	return actor;
}

export function createActivityStore(): {
	store: ReadStore<ActivityState>;
	writer: ActivityWriter;
} {
	const store = createStore<ActivityState>({
		entries: [],
		active: {},
		highlighted: [],
	});
	return {
		store: readOnly(store),
		writer: {
			record(entry, itemIds) {
				store.setState((s) => ({
					entries: [entry, ...s.entries].slice(0, MAX_ENTRIES),
					active: { ...s.active, [entry.actor]: entry.at },
					highlighted: [...new Set([...s.highlighted, ...itemIds])],
				}));
			},
			clearHighlights() {
				store.setState((s) =>
					s.highlighted.length === 0 ? s : { ...s, highlighted: [] },
				);
			},
		},
	};
}
