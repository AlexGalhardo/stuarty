import { router } from "@inertiajs/react";
import { useEffect, useId } from "react";

// Text typed and not saved yet is worth something: leaving it behind (another item, another page, closing the tab)
// asks first. The save stays a command to the core; this only remembers which editors hold unsaved text.
const dirtyEditors = new Set<string>();

export function hasUnsaved(): boolean {
	return dirtyEditors.size > 0;
}

export function markUnsaved(id: string, dirty: boolean): void {
	if (dirty) dirtyEditors.add(id);
	else dirtyEditors.delete(id);
}

// Asks once when something is unsaved; true means go on (nothing to lose, or the person agreed).
export function confirmDiscard(message: string): boolean {
	if (!hasUnsaved()) return true;
	return window.confirm(message);
}

function onBeforeUnload(event: BeforeUnloadEvent): void {
	if (!hasUnsaved()) return;
	event.preventDefault();
	// Chrome and Edge show their own dialog only when returnValue is set.
	event.returnValue = "";
}

let users = 0;
let question = "";
let off: (() => void) | null = null;

function attach(): void {
	window.addEventListener("beforeunload", onBeforeUnload);
	off = router.on("before", (event) => {
		if (!hasUnsaved()) return;
		// A visit to the same page (a reload after a save) is not a departure.
		if (event.detail.visit.url.pathname === window.location.pathname) return;
		if (!window.confirm(question)) event.preventDefault();
	});
}

function detach(): void {
	window.removeEventListener("beforeunload", onBeforeUnload);
	off?.();
	off = null;
}

// An editor calls this with whether it holds unsaved text. `message` is the question asked on a route change.
export function useUnsavedGuard(dirty: boolean, message: string): void {
	const id = useId();
	useEffect(() => {
		markUnsaved(id, dirty);
		return () => markUnsaved(id, false);
	}, [id, dirty]);

	useEffect(() => {
		question = message;
		if (users++ === 0) attach();
		return () => {
			if (--users === 0) detach();
		};
	}, [message]);
}
