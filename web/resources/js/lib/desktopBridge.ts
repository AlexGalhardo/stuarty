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
	// Opens the file manager on a stored library file; the shell checks the path against the media root of the
	// workspace by itself (SDD-011 section 14.10).
	revealAsset?: (path: string) => Promise<unknown>;
	getBackground?: () => Promise<unknown>;
	setBackground?: (settings: BackgroundSettings) => Promise<unknown>;
	// Opens and closes the teleprompter window of a project (SDD-025 section 20.3): the shell takes only the id of a
	// project and builds the address itself, and keeps the window out of a screen capture where the system allows it.
	openTeleprompter?: (project: string) => Promise<unknown>;
	closeTeleprompter?: () => Promise<unknown>;
	// The native menu calls an action id of the editor menu (SDD-012 section 6.4).
	onMenuAction?: (callback: (action: string) => void) => Unsubscribe;
	// Desktop in-app updater controls on Login and the sidebar footer (epic 101).
	getUpdateState?: () => Promise<unknown>;
	checkForUpdates?: () => Promise<unknown>;
	installUpdate?: () => Promise<unknown>;
	setAutoUpdate?: (enabled: boolean) => Promise<unknown>;
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

export function canOpenTeleprompter(bridge: DesktopBridge | null): boolean {
	return typeof bridge?.openTeleprompter === "function";
}

export function canRevealAsset(bridge: DesktopBridge | null): boolean {
	return typeof bridge?.revealAsset === "function";
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

// Opens the file manager on a stored library file, with the path the service returned for it. The shell checks that
// the path is a regular file under the media root before it opens anything, and the promise rejects otherwise.
export async function revealAsset(
	bridge: DesktopBridge | null,
	path: string,
): Promise<void> {
	await bridge?.revealAsset?.(path);
}

// The background mode of the shell (SDD-012 section 21.3 item 7): whether Stuarty stays resident behind a
// tray icon when the window closes with a post scheduled, and whether it starts hidden at sign-in. The
// shell keeps the choice; the page only shows and sends it. `error` is the reason the system refused the
// registration of the start at sign-in.
export interface BackgroundSettings {
	keep_running: boolean;
	start_at_sign_in: boolean;
	asked: boolean;
}

export interface BackgroundAnswer extends BackgroundSettings {
	error?: string;
}

export function canBackground(bridge: DesktopBridge | null): boolean {
	return (
		typeof bridge?.getBackground === "function" &&
		typeof bridge?.setBackground === "function"
	);
}

// The answer of the shell as typed settings, or null when it is not what the shell sends.
export function parseBackground(value: unknown): BackgroundAnswer | null {
	if (typeof value !== "object" || value === null) return null;
	const v = value as Record<string, unknown>;
	if (
		typeof v.keep_running !== "boolean" ||
		typeof v.start_at_sign_in !== "boolean" ||
		typeof v.asked !== "boolean"
	) {
		return null;
	}
	return {
		keep_running: v.keep_running,
		start_at_sign_in: v.start_at_sign_in,
		asked: v.asked,
		...(typeof v.error === "string" && v.error !== ""
			? { error: v.error }
			: {}),
	};
}

export async function readBackground(
	bridge: DesktopBridge | null,
): Promise<BackgroundAnswer | null> {
	if (!bridge?.getBackground) return null;
	return parseBackground(await bridge.getBackground());
}

export async function writeBackground(
	bridge: DesktopBridge | null,
	settings: BackgroundSettings,
): Promise<BackgroundAnswer | null> {
	if (!bridge?.setBackground) return null;
	return parseBackground(await bridge.setBackground(settings));
}

export function subscribeDesktopBridge(listener: () => void): Unsubscribe {
	window.addEventListener(DESKTOP_READY_EVENT, listener);
	return () => window.removeEventListener(DESKTOP_READY_EVENT, listener);
}

export type DesktopUpdateStatus =
	| "idle"
	| "checking"
	| "available"
	| "up_to_date"
	| "downloading"
	| "installing"
	| "offline"
	| "unconfigured"
	| "error";

const UPDATE_STATUSES: ReadonlySet<string> = new Set([
	"idle",
	"checking",
	"available",
	"up_to_date",
	"downloading",
	"installing",
	"offline",
	"unconfigured",
	"error",
]);

export interface DesktopUpdateState {
	current_version: string;
	available_version?: string;
	release_notes?: string;
	auto_update: boolean;
	status: DesktopUpdateStatus;
	progress?: number;
	error?: string;
}

export function canUpdate(bridge: DesktopBridge | null): boolean {
	return (
		typeof bridge?.getUpdateState === "function" &&
		typeof bridge?.checkForUpdates === "function" &&
		typeof bridge?.installUpdate === "function" &&
		typeof bridge?.setAutoUpdate === "function"
	);
}

export function parseUpdateState(value: unknown): DesktopUpdateState | null {
	if (typeof value !== "object" || value === null) return null;
	const v = value as Record<string, unknown>;
	if (
		typeof v.current_version !== "string" ||
		typeof v.auto_update !== "boolean" ||
		typeof v.status !== "string" ||
		!UPDATE_STATUSES.has(v.status)
	) {
		return null;
	}
	return {
		current_version: v.current_version,
		auto_update: v.auto_update,
		status: v.status as DesktopUpdateStatus,
		...(typeof v.available_version === "string" && v.available_version !== ""
			? { available_version: v.available_version }
			: {}),
		...(typeof v.release_notes === "string" && v.release_notes !== ""
			? { release_notes: v.release_notes }
			: {}),
		...(typeof v.progress === "number" ? { progress: v.progress } : {}),
		...(typeof v.error === "string" && v.error !== ""
			? { error: v.error }
			: {}),
	};
}

export async function readUpdateState(
	bridge: DesktopBridge | null,
): Promise<DesktopUpdateState | null> {
	if (!bridge?.getUpdateState) return null;
	return parseUpdateState(await bridge.getUpdateState());
}

export async function checkDesktopUpdates(
	bridge: DesktopBridge | null,
): Promise<DesktopUpdateState | null> {
	if (!bridge?.checkForUpdates) return null;
	return parseUpdateState(await bridge.checkForUpdates());
}

export async function installDesktopUpdate(
	bridge: DesktopBridge | null,
): Promise<DesktopUpdateState | null> {
	if (!bridge?.installUpdate) return null;
	return parseUpdateState(await bridge.installUpdate());
}

export async function writeAutoUpdate(
	bridge: DesktopBridge | null,
	enabled: boolean,
): Promise<DesktopUpdateState | null> {
	if (!bridge?.setAutoUpdate) return null;
	return parseUpdateState(await bridge.setAutoUpdate(enabled));
}
