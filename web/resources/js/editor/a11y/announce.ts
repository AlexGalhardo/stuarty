import { useSyncExternalStore } from "react";

// The announcements of SDD-012 section 18.1 item 5: a message is a catalogue
// key and its parameters, translated where it is read out (LiveRegion), so a
// change of language never leaves an old sentence in the region.
export interface Announcement {
	key: string;
	params?: Record<string, string | number>;
	// Changes with every announcement, so the same sentence twice is read twice.
	seq: number;
}

let current: Announcement | null = null;
let seq = 0;
const listeners = new Set<() => void>();
const lastByKey = new Map<string, number>();

export interface AnnounceOptions {
	// Drop the message when the same key was announced less than this long ago.
	minIntervalMs?: number;
	now?: () => number;
}

export function announce(
	key: string,
	params?: Record<string, string | number>,
	options: AnnounceOptions = {},
): boolean {
	const at = (options.now ?? Date.now)();
	const last = lastByKey.get(key);
	if (
		options.minIntervalMs !== undefined &&
		last !== undefined &&
		at - last < options.minIntervalMs
	) {
		return false;
	}
	lastByKey.set(key, at);
	seq += 1;
	current = { key, params, seq };
	for (const listener of listeners) listener();
	return true;
}

// For tests: forget every announcement and throttle.
export function resetAnnouncements(): void {
	current = null;
	seq = 0;
	lastByKey.clear();
	for (const listener of listeners) listener();
}

function subscribe(listener: () => void): () => void {
	listeners.add(listener);
	return () => listeners.delete(listener);
}

export function useAnnouncement(): Announcement | null {
	return useSyncExternalStore(
		subscribe,
		() => current,
		() => null,
	);
}
