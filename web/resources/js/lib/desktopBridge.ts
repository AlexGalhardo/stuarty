// The narrow bridge of the desktop shell (SDD-012 sections 13.1 item 2 and 21).
// The shell defines window.vukemDesktop; a plain browser has no such object and
// everything here reports "absent". Paths come from the native dialog or from a
// drop, and go to the same import API as a pasted path: the page never reads
// file bytes.

export type Unsubscribe = () => void;

export interface DesktopBridge {
	pickFiles?: () => Promise<string[]>;
	onDropPaths?: (callback: (paths: string[]) => void) => Unsubscribe;
	pickTemplate?: () => Promise<string[]>;
	showInFolder?: (path: string) => Promise<unknown>;
	// The native menu calls an action id of the editor menu (SDD-012 section 6.4).
	onMenuAction?: (callback: (action: string) => void) => Unsubscribe;
}

// The shell runs its script after the page loads, so the bridge can appear
// after the first render; it says so with this event.
export const DESKTOP_READY_EVENT = "vukem-desktop-ready";

declare global {
	interface Window {
		vukemDesktop?: DesktopBridge;
	}
}

function isBridge(value: unknown): value is DesktopBridge {
	return typeof value === "object" && value !== null;
}

// The bridge, or null in a plain browser (or outside a window).
export function getDesktopBridge(): DesktopBridge | null {
	if (typeof window === "undefined") return null;
	const bridge: unknown = window.vukemDesktop;
	return isBridge(bridge) ? bridge : null;
}

export function canPickFiles(bridge: DesktopBridge | null): boolean {
	return typeof bridge?.pickFiles === "function";
}

export function canPickTemplate(bridge: DesktopBridge | null): boolean {
	return typeof bridge?.pickTemplate === "function";
}

export function canDropPaths(bridge: DesktopBridge | null): boolean {
	return typeof bridge?.onDropPaths === "function";
}

export function canMenuAction(bridge: DesktopBridge | null): boolean {
	return typeof bridge?.onMenuAction === "function";
}

export function canShowInFolder(bridge: DesktopBridge | null): boolean {
	return typeof bridge?.showInFolder === "function";
}

// A list of absolute paths from the shell, with anything that is not a
// non-empty string dropped.
export function cleanPaths(value: unknown): string[] {
	if (!Array.isArray(value)) return [];
	return value.filter(
		(item): item is string => typeof item === "string" && item.trim() !== "",
	);
}

// Opens the file manager on an exported file; the shell refuses a path that is
// not an existing file, and the promise then rejects.
export async function showInFolder(
	bridge: DesktopBridge | null,
	path: string,
): Promise<void> {
	await bridge?.showInFolder?.(path);
}

export function subscribeDesktopBridge(listener: () => void): Unsubscribe {
	window.addEventListener(DESKTOP_READY_EVENT, listener);
	return () => window.removeEventListener(DESKTOP_READY_EVENT, listener);
}
