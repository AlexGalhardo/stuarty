import { endsAfter, startsAtOrAfter } from "../commands/units";

// Windowed rendering of a track (SDD-012 section 7.9). Pure: the read model
// and the UI state go in, the ids to mount come out.

// Above this many items in the project the rows mount only the window. The
// long-task test of SDD-012 section 19 decides the value.
export const VIRTUALIZE_ABOVE = 150;

// Width assumed before the first measure of the viewport.
const FALLBACK_VIEWPORT_PX = 1280;

export interface FrameWindow {
	from: number;
	to: number;
}

// `[scrollLeft - 1 viewport, scrollLeft + 2 viewports]` in frames. The edges
// are rounded outwards to an eighth of a viewport so that scrolling a few pixels
// yields the same window and the rows do not render at every scroll event.
export function frameWindow(
	scrollLeft: number,
	viewportWidth: number,
	pxPerFrame: number,
): FrameWindow {
	const viewport = viewportWidth > 0 ? viewportWidth : FALLBACK_VIEWPORT_PX;
	const quantum = viewport / 8;
	const fromPx =
		Math.floor(Math.max(0, scrollLeft - viewport) / quantum) * quantum;
	const toPx = Math.ceil((scrollLeft + 2 * viewport) / quantum) * quantum;
	return { from: fromPx / pxPerFrame, to: toPx / pxPerFrame };
}

interface Timed {
	id: string;
	start: number;
	duration: number;
}

// The ids to mount for the items of a track given in display order (by
// start). The core allows no overlap on a track, so the ends grow with the
// starts and both edges of the window are found by binary search. Items in
// `pinned` (selected, focused, dragged, being renamed) are always mounted.
// A null window mounts everything.
export function mountedIds(
	ordered: readonly Timed[],
	window: FrameWindow | null,
	pinned: ReadonlySet<string>,
): string[] {
	if (window === null) return ordered.map((item) => item.id);
	// First item that ends after the window starts.
	let low = 0;
	let high = ordered.length;
	while (low < high) {
		const mid = (low + high) >>> 1;
		if (endsAfter(ordered[mid], window.from)) high = mid;
		else low = mid + 1;
	}
	const first = low;
	// First item that starts at or after the window end.
	high = ordered.length;
	while (low < high) {
		const mid = (low + high) >>> 1;
		if (startsAtOrAfter(ordered[mid], window.to)) high = mid;
		else low = mid + 1;
	}
	const end = low;
	const ids: string[] = [];
	for (let i = 0; i < ordered.length; i++) {
		if ((i >= first && i < end) || pinned.has(ordered[i].id)) {
			ids.push(ordered[i].id);
		}
	}
	return ids;
}
