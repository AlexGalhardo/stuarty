import type { Asset, Item, Track } from "@/generated/timeline";

import type { DeepReadonly } from "./applyDiff";
import type { DocumentState } from "./documentStore";

// Narrow, memoized reads of the read model (SDD-012 section 5.2). They do no
// arithmetic on item times and contain no edit logic (rule T6); sorting items
// by start is display ordering and lives in timeline/geometry.ts.

type Doc = NonNullable<DocumentState["document"]>;

// One shared empty list: a selector that returns a new [] on every call makes
// useSyncExternalStore loop forever (React error 185).
const NO_IDS: readonly string[] = [];

const orderedCache = new WeakMap<object, readonly string[]>();
const byTrackCache = new WeakMap<
	object,
	ReadonlyMap<string, readonly string[]>
>();

// Tracks ordered by `order`, compared as byte strings (SDD-001 section 3.3.1).
export function selectOrderedTrackIds(state: DocumentState): readonly string[] {
	if (state.document === null) return NO_IDS;
	const tracks: Doc["tracks"] = state.document.tracks;
	let cached = orderedCache.get(tracks);
	if (cached === undefined) {
		cached = Object.values(tracks)
			.map((track) => ({ id: track.id, order: track.order }))
			.sort((a, b) => {
				if (a.order !== b.order) return a.order < b.order ? -1 : 1;
				return a.id < b.id ? -1 : 1;
			})
			.map((entry) => entry.id);
		orderedCache.set(tracks, cached);
	}
	return cached;
}

// The ids of the items of a track, in id order.
export function selectTrackItemIds(
	state: DocumentState,
	trackId: string,
): readonly string[] {
	if (state.document === null) return NO_IDS;
	const items: Doc["items"] = state.document.items;
	let grouped = byTrackCache.get(items);
	if (grouped === undefined) {
		const map = new Map<string, string[]>();
		for (const item of Object.values(items)) {
			const list = map.get(item.track);
			if (list) list.push(item.id);
			else map.set(item.track, [item.id]);
		}
		for (const list of map.values()) list.sort();
		grouped = map;
		byTrackCache.set(items, grouped);
	}
	return grouped.get(trackId) ?? NO_IDS;
}

export function selectItem(
	state: DocumentState,
	id: string,
): DeepReadonly<Item> | undefined {
	return state.document?.items[id];
}

export function selectTrack(
	state: DocumentState,
	id: string,
): DeepReadonly<Track> | undefined {
	return state.document?.tracks[id];
}

export function selectAsset(
	state: DocumentState,
	id: string,
): DeepReadonly<Asset> | undefined {
	return state.document?.assets[id];
}

// The end of the project as the core computed it, never derived here.
export function selectEndFrame(state: DocumentState): number {
	return state.endFrame;
}

export interface FrameRate {
	readonly num: number;
	readonly den: number;
}

// The project rate as written in the document, or undefined when the
// document leaves it to the schema default. The generated type of
// settings.fps is an intersection that json-schema-to-typescript could not
// reduce to {num, den}, hence the cast.
export function selectFps(state: DocumentState): FrameRate | undefined {
	return state.document?.settings.fps as unknown as FrameRate | undefined;
}
