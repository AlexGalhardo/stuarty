// The life cycle every pointer gesture of the timeline follows (SDD-012 section
// 8.1). It owns the pointer, the threshold, the keys that cancel and the scroll
// at the edges, and knows nothing about what a gesture edits.

export const DRAG_THRESHOLD_PX = 3;
export const EDGE_SCROLL_ZONE_PX = 40;
const EDGE_SCROLL_STEP_PX = 14;

export interface PointerSample {
	clientX: number;
	clientY: number;
	// Pointer travel since pointer down, in pixels, plus the horizontal scroll
	// done since then (the edge auto-scroll) so the frame under the pointer holds.
	dx: number;
	dy: number;
	alt: boolean;
}

export interface Gesture {
	// The pointer moved past the threshold: the gesture begins.
	start(): void;
	move(sample: PointerSample): void;
	// The pointer was released after the gesture began.
	end(sample: PointerSample): void | Promise<void>;
	// Esc or pointercancel before release: nothing is sent.
	cancel(): void;
}

let swallow = false;

// The click event that follows a drag must not select anything.
export function consumeDragClick(): boolean {
	const was = swallow;
	swallow = false;
	return was;
}

export interface Running {
	// Cancel from outside, for example a remote edit of the dragged item.
	cancel(): void;
}

export function runGesture(options: {
	event: { pointerId: number; clientX: number; clientY: number };
	target: Element;
	scroller: HTMLElement | null;
	gesture: Gesture;
}): Running {
	const { event, target, scroller, gesture } = options;
	const startScroll = scroller?.scrollLeft ?? 0;
	let began = false;
	let finished = false;
	let last: PointerSample = {
		clientX: event.clientX,
		clientY: event.clientY,
		dx: 0,
		dy: 0,
		alt: false,
	};
	let frame = 0;

	try {
		target.setPointerCapture(event.pointerId);
	} catch {
		// A synthetic event has no active pointer to capture.
	}

	const sample = (e: {
		clientX: number;
		clientY: number;
		altKey: boolean;
	}) => ({
		clientX: e.clientX,
		clientY: e.clientY,
		dx: e.clientX - event.clientX + ((scroller?.scrollLeft ?? 0) - startScroll),
		dy: e.clientY - event.clientY,
		alt: e.altKey,
	});

	const detach = (): void => {
		finished = true;
		cancelAnimationFrame(frame);
		window.removeEventListener("pointermove", onMove);
		window.removeEventListener("pointerup", onUp);
		window.removeEventListener("pointercancel", onCancel);
		window.removeEventListener("keydown", onKey, true);
		try {
			target.releasePointerCapture(event.pointerId);
		} catch {
			// Already released.
		}
	};

	const abort = (): void => {
		if (finished) return;
		detach();
		if (began) gesture.cancel();
	};

	function begin(): void {
		began = true;
		swallow = true;
		gesture.start();
		frame = requestAnimationFrame(tick);
	}

	// Scroll the timeline while the pointer is near its left or right edge.
	function tick(): void {
		if (finished) return;
		if (scroller !== null) {
			const rect = scroller.getBoundingClientRect();
			let step = 0;
			if (last.clientX < rect.left + EDGE_SCROLL_ZONE_PX) {
				step = -EDGE_SCROLL_STEP_PX;
			} else if (last.clientX > rect.right - EDGE_SCROLL_ZONE_PX) {
				step = EDGE_SCROLL_STEP_PX;
			}
			if (step !== 0) {
				const before = scroller.scrollLeft;
				scroller.scrollLeft = before + step;
				if (scroller.scrollLeft !== before) {
					last = sample({ ...last, altKey: last.alt });
					gesture.move(last);
				}
			}
		}
		frame = requestAnimationFrame(tick);
	}

	function onMove(e: PointerEvent): void {
		last = sample(e);
		if (!began) {
			if (Math.max(Math.abs(last.dx), Math.abs(last.dy)) < DRAG_THRESHOLD_PX) {
				return;
			}
			begin();
		}
		gesture.move(last);
	}

	function onUp(e: PointerEvent): void {
		if (finished) return;
		detach();
		if (began) {
			// The click that follows the release comes in the same task.
			setTimeout(forgetDragClick, 0);
			void gesture.end(sample(e));
		}
		// A click that never became a drag stays a click.
	}

	function onCancel(): void {
		abort();
	}

	// Esc cancels the gesture and is not seen by the shortcut that clears the
	// selection (SDD-012 section 7.10).
	function onKey(e: KeyboardEvent): void {
		if (e.key !== "Escape") return;
		if (began) e.stopPropagation();
		abort();
	}

	window.addEventListener("pointermove", onMove);
	window.addEventListener("pointerup", onUp);
	window.addEventListener("pointercancel", onCancel);
	window.addEventListener("keydown", onKey, true);

	return { cancel: abort };
}

// Forget a drag that ended without a click following it (released outside the
// element, for example).
export function forgetDragClick(): void {
	swallow = false;
}
