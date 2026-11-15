import type { Actions } from "../../commands/actions";
import { trimEdgeOperations } from "../../commands/builders";
import type { EditorRuntime } from "../../state/runtime";
import { deltaPxToFrames } from "../geometry";
import { snapFrame } from "../snapping";
import { runGesture } from "./lifecycle";
import { outlineConflict } from "./move";

// Dragging a trim handle (SDD-012 section 8.2): one `item.trim` to the frame the
// ghost shows. The ghost is clamped to the limits the core supplied with the
// snap points, for display only; the core still judges the request.

const WORDS_FROM_PPF = 2;

export function beginTrim(options: {
	runtime: EditorRuntime;
	actions: Actions;
	event: React.PointerEvent<HTMLElement>;
	itemId: string;
	edge: "start" | "end";
}): void {
	const { runtime, actions, event, itemId, edge } = options;
	const { documents, ui, ghostWriter, snap } = runtime;
	const doc = documents.getState().document;
	const baseRevision = documents.getState().revision;
	const item = doc?.items[itemId];
	if (
		doc === null ||
		item === undefined ||
		baseRevision === null ||
		documents.getState().state === "trashed"
	) {
		return;
	}
	const { start, duration, track } = item;
	const end = start + duration;
	const edgeFrame = edge === "start" ? start : end;
	const exclude = new Set([itemId]);
	let to = edgeFrame;

	runGesture({
		event,
		target: event.currentTarget,
		scroller: event.currentTarget.closest<HTMLElement>(
			"[data-timeline-scroller]",
		),
		gesture: {
			start() {
				void snap.begin(
					ui.getState().timeline.pxPerFrame >= WORDS_FROM_PPF &&
						item.type === "caption",
				);
			},
			move(sample) {
				const { pxPerFrame: ppf, snapEnabled } = ui.getState().timeline;
				const wanted = edgeFrame + deltaPxToFrames(sample.dx, ppf);
				const snapped = snapFrame({
					frame: wanted,
					ppf,
					playhead: ui.getState().playheadFrame,
					points: snap.points(),
					exclude,
					enabled: snapEnabled,
					suspended: sample.alt,
				});
				const limits = snap.limits(itemId);
				// One frame is the shortest an item can be.
				const low =
					edge === "start" ? (limits?.trim_start_min ?? 0) : start + 1;
				const high =
					edge === "start"
						? end - 1
						: (limits?.trim_end_max ?? Number.MAX_SAFE_INTEGER);
				to = Math.min(high, Math.max(low, snapped.frame));
				ghostWriter.setGhosts({
					[itemId]: {
						start: edge === "start" ? to : start,
						track,
						duration: edge === "start" ? end - to : to - start,
						pending: false,
					},
				});
				ghostWriter.setSnapLine(to === snapped.frame ? snapped.line : null);
			},
			async end() {
				if (to === edgeFrame) {
					ghostWriter.clear();
					return;
				}
				ghostWriter.setSnapLine(null);
				ghostWriter.markPending();
				const outcome = await actions.commitGesture(
					trimEdgeOperations(itemId, edge, to),
					baseRevision,
				);
				ghostWriter.clear();
				if (outcome !== null && !outcome.ok) {
					const conflicting = outcome.error.details?.conflicting_item;
					if (
						outcome.error.code === "E_OVERLAP" &&
						typeof conflicting === "string"
					) {
						outlineConflict(runtime, conflicting);
					}
				}
			},
			cancel() {
				ghostWriter.clear();
			},
		},
	});
}
