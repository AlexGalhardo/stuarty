import type { TrackPlacement } from "../../commands/builders";
import { type Gesture, runGesture } from "./lifecycle";

// The reorder drag of a track header (SDD-012 sections 7.4 and 8.1): a screen
// hit test over the header rows and nothing else. The result is one
// `track.move` naming the track the pointer is over; the new `order` is the
// core's (SDD-001 section 3.3.1).

export interface HeaderRect {
	id: string;
	top: number;
	bottom: number;
}

export interface DropTarget {
	placement: TrackPlacement;
	// The viewport y of the insertion line the ghost row is drawn at.
	lineY: number;
}

// Where releasing at `y` would put `draggedId`, or null when that is where it
// already is. `rects` are the header rows from the top, in display order.
export function dropTarget(
	rects: readonly HeaderRect[],
	draggedId: string,
	y: number,
): DropTarget | null {
	const first = rects[0];
	const last = rects[rects.length - 1];
	if (first === undefined || last === undefined) return null;
	const at = rects.findIndex((r) => r.id === draggedId);
	if (at < 0) return null;
	let target: DropTarget;
	if (y <= first.top) {
		target = { placement: { position: "top" }, lineY: first.top };
	} else if (y >= last.bottom) {
		target = { placement: { position: "bottom" }, lineY: last.bottom };
	} else {
		const index = rects.findIndex((r) => y >= r.top && y < r.bottom);
		const row = rects[index];
		if (row === undefined || row.id === draggedId) return null;
		target =
			y < (row.top + row.bottom) / 2
				? { placement: { above: row.id }, lineY: row.top }
				: { placement: { below: row.id }, lineY: row.bottom };
	}
	// A place next to the track itself changes nothing.
	const p = target.placement;
	if ("position" in p) {
		return (p.position === "top" && at === 0) ||
			(p.position === "bottom" && at === rects.length - 1)
			? null
			: target;
	}
	if ("above" in p && rects[at + 1]?.id === p.above) return null;
	if ("below" in p && rects[at - 1]?.id === p.below) return null;
	return target;
}

export interface Ghost {
	left: number;
	width: number;
	top: number;
	height: number;
}

function headerRects(): HeaderRect[] {
	return [
		...document.querySelectorAll<HTMLElement>("[data-header-for]"),
	].flatMap((el) => {
		const id = el.dataset.headerFor;
		if (id === undefined) return [];
		const box = el.getBoundingClientRect();
		return [{ id, top: box.top, bottom: box.bottom }];
	});
}

// The handle was pressed: past the threshold a ghost row follows the insertion
// line; on release one `track.move` is sent, Esc sends nothing.
export function beginReorder(options: {
	send(trackId: string, placement: TrackPlacement): void;
	event: PointerEvent | React.PointerEvent;
	trackId: string;
	onGhost(ghost: Ghost | null): void;
}): void {
	const { send, event, trackId, onGhost } = options;
	const target = event.currentTarget as Element;
	let rects: HeaderRect[] = [];
	let box: DOMRect | null = null;
	const gesture: Gesture = {
		start() {
			rects = headerRects();
			const own = document.querySelector<HTMLElement>(
				`[data-header-for="${CSS.escape(trackId)}"]`,
			);
			box = own?.getBoundingClientRect() ?? null;
		},
		move(sample) {
			const drop = dropTarget(rects, trackId, sample.clientY);
			if (drop === null || box === null) return onGhost(null);
			onGhost({
				left: box.left,
				width: box.width,
				top: drop.lineY - box.height / 2,
				height: box.height,
			});
		},
		end(sample) {
			onGhost(null);
			const drop = dropTarget(rects, trackId, sample.clientY);
			if (drop !== null) send(trackId, drop.placement);
		},
		cancel() {
			onGhost(null);
		},
	};
	runGesture({
		event,
		target,
		scroller: null,
		gesture,
		onFinish: () => onGhost(null),
	});
}
