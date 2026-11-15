import type { Actions } from "../../commands/actions";
import {
	type TransformChanges,
	transformOperations,
} from "../../commands/builders";
import { useActions } from "../../commands/useActions";
import {
	distance,
	nextRotation,
	nextScale,
	pointerAngle,
	previewGeometry,
	round3,
	turnBetween,
} from "../../preview/overlays/transformGestures";
import type { CanvasPort, TransformBegin } from "../../preview/types";
import { useRuntime } from "../../state/EditorContext";
import type { EditorRuntime } from "../../state/runtime";
import { consumeDragClick, runGesture, watchRemoteEdit } from "./lifecycle";

// The canvas gestures of SDD-012 sections 8.2 and 11.6. While the pointer moves
// only the outline box follows it (`onPreview`); the picture and the compositor
// are not touched. On release one batch is sent and the canvas changes when the
// plan window of the new revision arrives (rule T6).

const SETTLE_MS = 2000;

function beginTransform(
	runtime: EditorRuntime,
	actions: Actions,
	options: TransformBegin,
): void {
	const { event, kind, itemId, start, scale, overlay } = options;
	const { documents, ui } = runtime;
	const baseRevision = documents.getState().revision;
	const item = documents.getState().document?.items[itemId];
	if (
		item === undefined ||
		baseRevision === null ||
		scale <= 0 ||
		documents.getState().state === "trashed"
	) {
		return;
	}
	const origin = overlay.getBoundingClientRect();
	const centre = {
		x: origin.left + start.cx * scale,
		y: origin.top + start.cy * scale,
	};
	const down = { x: event.clientX, y: event.clientY };
	const downDistance = Math.max(distance(centre, down), 1);
	const downAngle = pointerAngle(centre, down);
	const keyframes = item.keyframes ?? {};
	// The scale the plan reports at the playhead, keyframes applied.
	const baseScale = start.scale;

	let changes: TransformChanges | null = null;
	let stopWatching: () => void = () => {};
	runGesture({
		event,
		target: event.currentTarget,
		scroller: null,
		gesture: {
			start() {
				stopWatching = watchRemoteEdit(runtime, [itemId], () => {
					options.onPreview(null);
				});
			},
			move(sample) {
				const pointer = { x: sample.clientX, y: sample.clientY };
				const factor = distance(centre, pointer) / downDistance;
				const turn = turnBetween(downAngle, pointerAngle(centre, pointer));
				const dx = sample.dx / scale;
				const dy = sample.dy / scale;
				options.onPreview(
					previewGeometry(start, kind, {
						dx,
						dy,
						factor: nextScale(baseScale, factor) / baseScale,
						turn,
					}),
				);
				if (kind === "body") {
					changes = { x: round3(start.cx + dx), y: round3(start.cy + dy) };
				} else if (kind === "scale") {
					changes = { scale: nextScale(baseScale, factor) };
				} else {
					changes = { rotation: nextRotation(start.rotation, turn) };
				}
			},
			async end() {
				stopWatching();
				const sent = changes;
				// The box stays where it was released until the answer; a refusal
				// puts it back, a success is followed by the new plan window.
				if (sent === null) {
					options.onPreview(null);
					return;
				}
				const ops = transformOperations(
					[{ id: itemId, animated: Object.keys(keyframes) }],
					sent,
					ui.getState().playheadFrame,
				);
				const outcome = await actions.commitGesture(ops, baseRevision);
				if (outcome === null || !outcome.ok) {
					options.onPreview(null);
					return;
				}
				// The caller drops the box when the plan window of the new
				// revision arrives; a batch that changed nothing never does.
				setTimeout(() => {
					if ((documents.getState().revision ?? 0) <= baseRevision) {
						options.onPreview(null);
					}
				}, SETTLE_MS);
			},
			cancel() {
				stopWatching();
				options.onPreview(null);
			},
		},
	});
}

// The port the layout gives the preview: the gesture, the click guard and the
// commit of the inline text editor.
export function useCanvasPort(): CanvasPort {
	const runtime = useRuntime();
	const actions = useActions();
	return {
		begin: (options) => beginTransform(runtime, actions, options),
		consumeClick: consumeDragClick,
		commitText: (itemId, text) => void actions.setText(itemId, text),
	};
}
