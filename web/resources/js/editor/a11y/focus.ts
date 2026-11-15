// F6 and Shift+F6 move focus between the regions of the editor (SDD-012
// section 18.2): the elements that carry `data-landmark`, in document order
// (top bar, library, preview, timeline, inspector). A region that is not on
// screen is skipped.
export const LANDMARK_SELECTOR = "[data-landmark]";

export function landmarks(root: ParentNode = document): HTMLElement[] {
	return [...root.querySelectorAll<HTMLElement>(LANDMARK_SELECTOR)];
}

// The region that holds `active`, or -1.
export function currentLandmark(
	list: readonly HTMLElement[],
	active: Element | null,
): number {
	if (active === null) return -1;
	return list.findIndex((el) => el === active || el.contains(active));
}

export function jumpLandmark(
	direction: 1 | -1,
	root: ParentNode = document,
	active: Element | null = document.activeElement,
): HTMLElement | null {
	const list = landmarks(root);
	if (list.length === 0) return null;
	const from = currentLandmark(list, active);
	const start = from === -1 ? (direction === 1 ? -1 : 0) : from;
	const next = list[(start + direction + list.length) % list.length];
	if (next === undefined) return null;
	if (!next.hasAttribute("tabindex")) next.tabIndex = -1;
	next.focus();
	return next;
}
