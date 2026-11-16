import type { Actions } from "../../commands/actions";
import {
	boundaryWords,
	captionWordsOperations,
	clampBoundary,
} from "../../commands/builders";
import type { EditorRuntime } from "../../state/runtime";
import { deltaPxToFrames } from "../geometry";
import { snapFrame } from "../snapping";
import { runGesture, watchRemoteEdit } from "./lifecycle";

// Dragging the boundary in front of a caption word (SDD-012 section 8.2): one
// `caption.set_words` with the two neighbouring words adjusted and the others
// copied from the read model. Only the ghost boundary (the snap line) follows
// the pointer; the words change when the new revision arrives.
export function beginWordBoundary(options: {
	runtime: EditorRuntime;
	actions: Actions;
	event: React.PointerEvent<HTMLElement>;
	itemId: string;
	// The word the boundary is in front of; the one before it gives way too.
	index: number;
}): void {
	const { runtime, actions, event, itemId, index } = options;
	const { documents, ui, ghostWriter, snap } = runtime;
	const baseRevision = documents.getState().revision;
	const item = documents.getState().document?.items[itemId];
	const words = item?.words;
	if (
		item === undefined ||
		words === undefined ||
		words[index] === undefined ||
		baseRevision === null ||
		documents.getState().state === "trashed"
	) {
		return;
	}
	const origin = words[index].start;
	const exclude = new Set([itemId]);
	let to = origin;

	let stopWatching: () => void = () => {};
	const running = runGesture({
		event,
		target: event.currentTarget,
		scroller: event.currentTarget.closest<HTMLElement>(
			"[data-timeline-scroller]",
		),
		gesture: {
			start() {
				void snap.begin(true);
			},
			move(sample) {
				const { pxPerFrame: ppf, snapEnabled } = ui.getState().timeline;
				const wanted = item.start + origin + deltaPxToFrames(sample.dx, ppf);
				const snapped = snapFrame({
					frame: wanted,
					ppf,
					playhead: ui.getState().playheadFrame,
					points: snap.points(),
					exclude,
					enabled: snapEnabled,
					suspended: sample.alt || sample.mod,
				});
				to = clampBoundary(words, index, snapped.frame - item.start);
				ghostWriter.setSnapLine(item.start + to);
			},
			async end() {
				ghostWriter.setSnapLine(null);
				if (to === origin) {
					ghostWriter.clear();
					return;
				}
				await actions.commitGesture(
					captionWordsOperations(itemId, boundaryWords(words, index, to)),
					baseRevision,
				);
				ghostWriter.clear();
			},
			cancel() {
				ghostWriter.clear();
			},
		},
		onFinish: () => stopWatching(),
	});
	stopWatching = watchRemoteEdit(runtime, [itemId], (actor) => {
		if (!running.dragging()) return;
		running.cancel();
		runtime.noticeWriter.push({
			kind: "info",
			message: "",
			messageKey: "ui.editor.notice.gesture_cancelled",
			actor,
		});
	});
}
