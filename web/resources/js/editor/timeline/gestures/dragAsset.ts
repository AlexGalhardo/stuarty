import type { MediaEntry } from "../../api/types";
import type { Actions } from "../../commands/actions";
import { dropAssetOperations } from "../../commands/builders";
import { useActions } from "../../commands/useActions";
import { useRuntime } from "../../state/EditorContext";
import type { EditorRuntime } from "../../state/runtime";
import { selectFps, selectTrack } from "../../state/selectors";
import { clientXToFrame, DEFAULT_FPS, framesPerSecond } from "../geometry";
import { runGesture } from "./lifecycle";

// Dragging a library card onto a track (SDD-012 section 8.2): the ghost has the
// duration `listMedia` gave, the release sends `clip.add` or `audio.add` with
// the row under the pointer and the frame under it. Screen arithmetic only; the
// core decides whether the place is free and the track the right kind.

interface Drop {
	trackId: string;
	frame: number;
}

function rowUnder(clientX: number, clientY: number, ppf: number): Drop | null {
	for (const el of document.elementsFromPoint(clientX, clientY)) {
		if (!(el instanceof HTMLElement)) continue;
		const trackId = el.dataset.trackId;
		if (trackId === undefined || el.getAttribute("role") !== "row") continue;
		const left = el.getBoundingClientRect().left;
		return { trackId, frame: clientXToFrame(clientX, left, 0, ppf) };
	}
	return null;
}

export function beginAssetDrag(options: {
	runtime: EditorRuntime;
	actions: Actions;
	event: React.PointerEvent<HTMLElement>;
	entry: MediaEntry;
}): void {
	const { runtime, actions, event, entry } = options;
	const { documents, ui, ghostWriter } = runtime;
	const baseRevision = documents.getState().revision;
	if (
		baseRevision === null ||
		documents.getState().state === "trashed" ||
		entry.state !== "ready" ||
		documents.getState().document?.assets[entry.asset_id] === undefined
	) {
		return;
	}
	const fps = selectFps(documents.getState()) ?? DEFAULT_FPS;
	const duration = entry.duration_frames ?? framesPerSecond(fps);
	const key = `asset:${entry.asset_id}`;
	let drop: Drop | null = null;

	runGesture({
		event,
		target: event.currentTarget,
		scroller: document.querySelector<HTMLElement>("[data-timeline-scroller]"),
		gesture: {
			start() {},
			move(sample) {
				drop = rowUnder(
					sample.clientX,
					sample.clientY,
					ui.getState().timeline.pxPerFrame,
				);
				ghostWriter.setGhosts(
					drop === null
						? {}
						: {
								[key]: {
									start: drop.frame,
									track: drop.trackId,
									duration,
									pending: false,
								},
							},
				);
			},
			async end() {
				const target = drop;
				const track =
					target === null
						? undefined
						: selectTrack(documents.getState(), target.trackId);
				if (target === null || track === undefined) {
					ghostWriter.clear();
					return;
				}
				ghostWriter.markPending();
				await actions.commitGesture(
					dropAssetOperations({
						assetId: entry.asset_id,
						assetKind: entry.kind,
						trackId: target.trackId,
						trackKind: track.kind,
						start: target.frame,
					}),
					baseRevision,
				);
				ghostWriter.clear();
			},
			cancel() {
				ghostWriter.clear();
			},
		},
	});
}

// The handler the library gets from the layout: the library folder does not
// import the timeline (SDD-012 section 3.3), so the gesture is handed in.
export function useAssetDrag(): (
	event: React.PointerEvent<HTMLElement>,
	entry: MediaEntry,
) => void {
	const runtime = useRuntime();
	const actions = useActions();
	return (event, entry) => beginAssetDrag({ runtime, actions, event, entry });
}
