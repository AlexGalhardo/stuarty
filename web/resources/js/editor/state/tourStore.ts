import { createStore, readOnly, useStore } from "./createStore";
import { readShortcutsSeen, writeShortcutsSeen } from "./prefs";

// The first-run tour (SDD-012 section 18.5): whether it is on screen and which
// step. It starts open when this browser has not seen it; "Replay the tour"
// starts it at the first step whatever was remembered. Showing it is what
// remembers it, so a reload never shows it again, finished or skipped.
export interface TourState {
	open: boolean;
	step: number;
}

const store = createStore<TourState>({ open: !readShortcutsSeen(), step: 0 });
export const tourStore = readOnly(store);

export function useTour(): TourState {
	return useStore(tourStore, (s) => s);
}

export function rememberTour(): void {
	writeShortcutsSeen();
}

export function startTour(): void {
	store.setState(() => ({ open: true, step: 0 }));
}

export function goToTourStep(step: number): void {
	store.setState((state) => ({ ...state, step: Math.max(step, 0) }));
}

export function endTour(): void {
	store.setState(() => ({ open: false, step: 0 }));
}
