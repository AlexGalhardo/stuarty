import { type RefObject, useCallback, useRef } from "react";

// The marquee of SDD-012 section 7.6: a screen hit test and nothing else. It
// selects the items whose blocks the dragged box touches; the result is a list
// of ids read from the DOM, never a value computed from the timeline.

export const MARQUEE_THRESHOLD_PX = 3;

export interface Point {
	x: number;
	y: number;
}

export interface Box {
	left: number;
	top: number;
	right: number;
	bottom: number;
}

export function boxBetween(a: Point, b: Point): Box {
	return {
		left: Math.min(a.x, b.x),
		top: Math.min(a.y, b.y),
		right: Math.max(a.x, b.x),
		bottom: Math.max(a.y, b.y),
	};
}

export function pastThreshold(a: Point, b: Point): boolean {
	return (
		Math.max(Math.abs(a.x - b.x), Math.abs(a.y - b.y)) >= MARQUEE_THRESHOLD_PX
	);
}

export function touches(box: Box, rect: Box): boolean {
	return (
		box.left <= rect.right &&
		box.right >= rect.left &&
		box.top <= rect.bottom &&
		box.bottom >= rect.top
	);
}

export function hitItems(
	box: Box,
	entries: readonly { id: string; rect: Box }[],
): string[] {
	return entries.filter((e) => touches(box, e.rect)).map((e) => e.id);
}

function itemRects(root: HTMLElement): { id: string; rect: Box }[] {
	return [...root.querySelectorAll<HTMLElement>("[data-item-id]")].flatMap(
		(el) => {
			const id = el.dataset.itemId;
			return id === undefined ? [] : [{ id, rect: el.getBoundingClientRect() }];
		},
	);
}

// Pointer handling for the scroll area: pointerdown on empty space (not on an
// item) arms the marquee, a move past the threshold draws the box in `boxRef`
// (written straight to its style, so no React render runs at pointer rate) and
// release selects what it touches. `consumeClick` tells the click that follows
// a marquee not to clear the selection.
export function useMarquee(
	root: RefObject<HTMLElement | null>,
	boxRef: RefObject<HTMLElement | null>,
	onSelect: (ids: string[]) => void,
): {
	onPointerDown(event: React.PointerEvent<HTMLElement>): void;
	consumeClick(): boolean;
} {
	const swallowClick = useRef(false);

	const onPointerDown = useCallback(
		(event: React.PointerEvent<HTMLElement>) => {
			const area = root.current;
			if (
				area === null ||
				event.button !== 0 ||
				!(event.target instanceof Element) ||
				event.target.closest("[data-item-id]") !== null
			) {
				return;
			}
			const origin: Point = { x: event.clientX, y: event.clientY };
			let active = false;

			const draw = (box: Box): void => {
				const el = boxRef.current;
				const base = area.getBoundingClientRect();
				if (el === null) return;
				el.style.display = "block";
				el.style.left = `${box.left - base.left + area.scrollLeft}px`;
				el.style.top = `${box.top - base.top + area.scrollTop}px`;
				el.style.width = `${box.right - box.left}px`;
				el.style.height = `${box.bottom - box.top}px`;
			};
			const move = (e: PointerEvent): void => {
				const here = { x: e.clientX, y: e.clientY };
				if (!active && !pastThreshold(origin, here)) return;
				active = true;
				const box = boxBetween(origin, here);
				draw(box);
				onSelect(hitItems(box, itemRects(area)));
			};
			const end = (): void => {
				window.removeEventListener("pointermove", move);
				window.removeEventListener("pointerup", end);
				window.removeEventListener("pointercancel", end);
				if (boxRef.current !== null) boxRef.current.style.display = "none";
				if (active) {
					swallowClick.current = true;
					setTimeout(() => {
						swallowClick.current = false;
					}, 0);
				}
			};
			window.addEventListener("pointermove", move);
			window.addEventListener("pointerup", end);
			window.addEventListener("pointercancel", end);
		},
		[root, boxRef, onSelect],
	);

	const consumeClick = useCallback((): boolean => {
		const was = swallowClick.current;
		swallowClick.current = false;
		return was;
	}, []);

	return { onPointerDown, consumeClick };
}
