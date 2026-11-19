import type { DocumentState } from "../state/documentStore";
import { selectOrderedTrackIds, selectTrackItemIds } from "../state/selectors";
import { nearestByStart, sortByStart } from "./geometry";

// The selection model and the keyboard grid of SDD-012 sections 7.6 and 7.10, as
// pure functions over a layout the components read from the document. No
// arithmetic on item times here (rule R5): ordering and distance come from
// geometry.ts, the nudge from commands/builders.ts.

export interface GridItem {
	id: string;
	start: number;
	duration: number;
}

export interface GridTrack {
	id: string;
	items: readonly GridItem[];
}

// Tracks in display order, each with its items sorted by start.
export type GridLayout = readonly GridTrack[];

export function gridLayout(state: DocumentState): GridLayout {
	const doc = state.document;
	if (doc === null) return [];
	return selectOrderedTrackIds(state).map((id) => ({
		id,
		items: sortByStart(
			selectTrackItemIds(state, id).flatMap((itemId) => {
				const item = doc.items[itemId];
				return item === undefined
					? []
					: [{ id: itemId, start: item.start, duration: item.duration }];
			}),
		),
	}));
}

export function allItemIds(layout: GridLayout): string[] {
	return layout.flatMap((track) => track.items.map((item) => item.id));
}

function locate(
	layout: GridLayout,
	id: string,
): { track: number; index: number } | null {
	for (const [t, track] of layout.entries()) {
		const index = track.items.findIndex((item) => item.id === id);
		if (index >= 0) return { track: t, index };
	}
	return null;
}

export type Direction = "left" | "right" | "up" | "down" | "home" | "end";

// The item the focus moves to, or null when it stays.
export function focusTarget(
	layout: GridLayout,
	focused: string | null,
	direction: Direction,
): string | null {
	const at = focused === null ? null : locate(layout, focused);
	if (at === null) return allItemIds(layout)[0] ?? null;
	const track = layout[at.track];
	if (track === undefined) return null;
	switch (direction) {
		case "left":
			return track.items[at.index - 1]?.id ?? null;
		case "right":
			return track.items[at.index + 1]?.id ?? null;
		case "home":
			return track.items[0]?.id ?? null;
		case "end":
			return track.items.at(-1)?.id ?? null;
		case "up":
		case "down": {
			const step = direction === "up" ? -1 : 1;
			const from = track.items[at.index];
			if (from === undefined) return null;
			// The nearest track in that direction that has an item.
			for (let t = at.track + step; t >= 0 && t < layout.length; t += step) {
				const candidates = layout[t]?.items ?? [];
				if (candidates.length > 0)
					return nearestByStart(candidates, from.start);
			}
			return null;
		}
	}
}

// Where focus goes when the focused item left the read model: the next item on
// its track, else the previous one, else the first item anywhere, else null
// (the grid itself keeps the focus).
export function focusAfterRemoval(
	previous: GridLayout,
	removed: string,
	current: GridLayout,
): string | null {
	const known = new Set(allItemIds(current));
	const at = locate(previous, removed);
	if (at !== null) {
		const items = previous[at.track]?.items ?? [];
		const after = items.slice(at.index + 1).find((i) => known.has(i.id));
		if (after !== undefined) return after.id;
		const before = items
			.slice(0, at.index)
			.reverse()
			.find((i) => known.has(i.id));
		if (before !== undefined) return before.id;
	}
	return allItemIds(current)[0] ?? null;
}

// The track whose header takes the focus when the removed item was the last
// one on its track and the track is still there (next on
// the track, else previous, else the header); null when an item can take it.
export function headerAfterRemoval(
	previous: GridLayout,
	removed: string,
	current: GridLayout,
): string | null {
	const at = locate(previous, removed);
	if (at === null) return null;
	const trackId = previous[at.track]?.id;
	const now = current.find((track) => track.id === trackId);
	return now !== undefined && now.items.length === 0 ? now.id : null;
}

// The item that holds the tab stop: the focused one if it still exists, else
// the first item of the first track that has one.
export function tabStop(
	layout: GridLayout,
	focused: string | null,
): string | null {
	if (focused !== null && locate(layout, focused) !== null) return focused;
	return allItemIds(layout)[0] ?? null;
}

// The items between two items of the same track, both included, in display
// order. When the anchor is on another track the range is the clicked item
// alone.
export function rangeOnTrack(
	layout: GridLayout,
	anchor: string | null,
	clicked: string,
): string[] {
	const to = locate(layout, clicked);
	const from = anchor === null ? null : locate(layout, anchor);
	if (to === null) return [];
	if (from === null || from.track !== to.track) return [clicked];
	const items = layout[to.track]?.items ?? [];
	const low = Math.min(from.index, to.index);
	const high = Math.max(from.index, to.index);
	return items.slice(low, high + 1).map((i) => i.id);
}

export interface ClickModifiers {
	shift: boolean;
	toggle: boolean;
}

export interface SelectionResult {
	items: string[];
	anchor: string | null;
}

// A click on an item (SDD-012 section 7.6): plain selects it, Shift adds the
// range from the anchor on the same track, Ctrl or Cmd toggles it.
export function clickSelection(
	layout: GridLayout,
	current: readonly string[],
	anchor: string | null,
	clicked: string,
	modifiers: ClickModifiers,
): SelectionResult {
	if (modifiers.toggle) {
		const items = current.includes(clicked)
			? current.filter((id) => id !== clicked)
			: [...current, clicked];
		return { items, anchor: clicked };
	}
	if (modifiers.shift) {
		const items = rangeOnTrack(layout, anchor, clicked);
		return { items, anchor: items.length > 1 ? anchor : clicked };
	}
	return { items: [clicked], anchor: clicked };
}

// Shift+Left and Shift+Right: the range from the anchor to the new focus.
export function extendSelection(
	layout: GridLayout,
	anchor: string | null,
	focus: string,
): SelectionResult {
	const from = anchor ?? focus;
	return { items: rangeOnTrack(layout, from, focus), anchor: from };
}

export type GridCommand =
	| { kind: "focus"; direction: Direction; extend: boolean }
	| { kind: "activate" }
	| { kind: "nudge"; unit: "frame" | "second"; sign: 1 | -1 }
	| { kind: "move-track"; sign: 1 | -1 };

type KeyEvent = Pick<
	KeyboardEvent,
	"key" | "ctrlKey" | "metaKey" | "altKey" | "shiftKey"
> &
	Partial<Pick<KeyboardEvent, "code">>;

const FOCUS_KEYS: Readonly<Record<string, Direction>> = {
	ArrowLeft: "left",
	ArrowRight: "right",
	ArrowUp: "up",
	ArrowDown: "down",
	Home: "home",
	End: "end",
};

// The grid keys of SDD-012 section 7.10: the arrows move the focus, Alt with
// the arrows nudges or moves to another track, Enter activates. They are
// widget keys of the focused cell, not rows of the registry (the `grid_keys`
// row lists them); every other key of the timeline is a row (SDD-012 section
// 18.9). Space is never taken: it always toggles play.
export function gridCommand(event: KeyEvent): GridCommand | null {
	// Mod with the arrows is the registry's (the markers, slide and roll).
	if (event.ctrlKey || event.metaKey) return null;
	if (event.altKey) {
		const unit = event.shiftKey ? "second" : "frame";
		if (event.key === "ArrowLeft") return { kind: "nudge", unit, sign: -1 };
		if (event.key === "ArrowRight") return { kind: "nudge", unit, sign: 1 };
		if (event.shiftKey) return null;
		if (event.key === "ArrowUp") return { kind: "move-track", sign: -1 };
		if (event.key === "ArrowDown") return { kind: "move-track", sign: 1 };
		return null;
	}
	if (event.key === "Enter") {
		return event.shiftKey ? null : { kind: "activate" };
	}
	const direction = FOCUS_KEYS[event.key];
	if (direction === undefined) return null;
	if (event.shiftKey) {
		return direction === "left" || direction === "right"
			? { kind: "focus", direction, extend: true }
			: null;
	}
	return { kind: "focus", direction, extend: false };
}
