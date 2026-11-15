import type { EditorRuntime } from "../state/runtime";
import { selectFps } from "../state/selectors";
import { DEFAULT_FPS } from "../timeline/geometry";
import { revealItem } from "../timeline/revealItem";

// Deep links of SDD-012 section 2.2: `?t=<time>` puts the playhead there and
// `?item=itm_...` selects an item and scrolls it into view. The URL is kept in
// step with the UI with history.replaceState, so a reload or a second tab
// opens the same place.

const ITEM_ID = /^itm_[0-9a-hjkmnp-tv-z]{10}$/;
const REST_MS = 1000;

export interface DeepLink {
	// The playhead frame, or null when the link has none or it is not a time.
	frame: number | null;
	item: string | null;
}

// A time as SDD-000 writes it: "375f", "12.5s" or "00:00:12.500". The frame of
// a time in seconds is the one the playhead shows at that time: floor.
export function parseTime(
	text: string,
	fps: { num: number; den: number },
): number | null {
	const frames = /^(\d+)f$/.exec(text);
	if (frames) return Number(frames[1]);
	const seconds = /^(\d+(?:\.\d+)?)s$/.exec(text);
	if (seconds) return Math.floor((Number(seconds[1]) * fps.num) / fps.den);
	const clock = /^(\d+):([0-5]\d):([0-5]\d(?:\.\d+)?)$/.exec(text);
	if (clock) {
		const total =
			Number(clock[1]) * 3600 + Number(clock[2]) * 60 + Number(clock[3]);
		return Math.floor((total * fps.num) / fps.den);
	}
	return null;
}

export function parseDeepLink(
	search: string,
	fps: { num: number; den: number },
): DeepLink {
	const params = new URLSearchParams(search);
	const t = params.get("t");
	const item = params.get("item");
	return {
		frame: t === null ? null : parseTime(t, fps),
		item: item !== null && ITEM_ID.test(item) ? item : null,
	};
}

// The query for the current place: the frame is written as `<n>f`, which is
// exact and needs no division.
export function deepLinkQuery(frame: number, item: string | null): string {
	const params = new URLSearchParams();
	if (frame > 0) params.set("t", `${frame}f`);
	if (item !== null) params.set("item", item);
	const text = params.toString();
	return text === "" ? "" : `?${text}`;
}

export interface DeepLinkDeps {
	search: string;
	replaceUrl(query: string): void;
	scrollIntoView(itemId: string): void;
	setTimeout(callback: () => void, ms: number): unknown;
	clearTimeout(handle: unknown): void;
}

export const browserDeepLinkDeps = (): DeepLinkDeps => ({
	search: location.search,
	replaceUrl: (query) =>
		history.replaceState(
			history.state,
			"",
			`${location.pathname}${query}${location.hash}`,
		),
	scrollIntoView: (itemId) =>
		revealItem(itemId, { block: "nearest", inline: "center" }),
	setTimeout: (callback, ms) => setTimeout(callback, ms),
	clearTimeout: (handle) => clearTimeout(handle as number),
});

// Applies the link once the document is there, then keeps the URL in step.
// Returns the function that stops it.
export function startDeepLink(
	runtime: EditorRuntime,
	deps: DeepLinkDeps = browserDeepLinkDeps(),
): () => void {
	let applied = false;
	let rest: unknown = null;
	let offDocument: (() => void) | null = null;
	let offUi: (() => void) | null = null;

	function write(): void {
		const { playheadFrame, selection } = runtime.ui.getState();
		deps.replaceUrl(deepLinkQuery(playheadFrame, selection.items[0] ?? null));
	}

	function follow(): void {
		let frame = runtime.ui.getState().playheadFrame;
		let selected = runtime.ui.getState().selection.items[0];
		offUi = runtime.ui.subscribe((state) => {
			if (state.selection.items[0] !== selected) {
				// A selection is a decision: written at once.
				selected = state.selection.items[0];
				write();
			}
			if (state.playheadFrame !== frame) {
				// The playhead is written when it rests.
				frame = state.playheadFrame;
				if (rest !== null) deps.clearTimeout(rest);
				rest = deps.setTimeout(() => {
					rest = null;
					write();
				}, REST_MS);
			}
		});
	}

	function apply(): void {
		const state = runtime.documents.getState();
		if (applied || state.status !== "ready" || state.document === null) return;
		applied = true;
		const link = parseDeepLink(deps.search, selectFps(state) ?? DEFAULT_FPS);
		if (link.frame !== null) runtime.uiWriter.setPlayhead(link.frame);
		if (link.item !== null && state.document.items[link.item] !== undefined) {
			runtime.uiWriter.select([link.item]);
			// The block exists after the render that follows the selection.
			deps.setTimeout(() => deps.scrollIntoView(link.item as string), 0);
		}
		follow();
	}

	apply();
	if (!applied) offDocument = runtime.documents.subscribe(apply);

	return () => {
		offDocument?.();
		offUi?.();
		if (rest !== null) deps.clearTimeout(rest);
	};
}
