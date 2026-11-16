import type { Actions } from "../../commands/actions";
import { type MoveTarget, moveItems } from "../../commands/builders";
import type { Ghost } from "../../state/ghostStore";
import type { EditorRuntime } from "../../state/runtime";
import { deltaPxToFrames } from "../geometry";
import { revealItem } from "../revealItem";
import { snapFrame } from "../snapping";
import { runGesture, watchRemoteEdit } from "./lifecycle";

// Dragging item bodies (SDD-012 section 8.2): horizontally, to another track,
// or as a multi-selection with the same delta. Screen arithmetic and snapping
// only: the ghost shows the requested place, the release sends the request and
// the core decides (rules T3 to T5).

export const CONFLICT_OUTLINE_MS = 2000;
// Caption words are snap targets only at the zoom where the timeline draws them.
const WORDS_FROM_PPF = 2;

interface Dragged {
	id: string;
	start: number;
	duration: number;
	track: string;
}

function trackUnder(clientX: number, clientY: number): string | null {
	for (const el of document.elementsFromPoint(clientX, clientY)) {
		const id = el instanceof HTMLElement ? el.dataset.trackId : undefined;
		if (id !== undefined) return id;
	}
	return null;
}

export function beginMove(options: {
	runtime: EditorRuntime;
	actions: Actions;
	event: React.PointerEvent<HTMLElement>;
	itemId: string;
}): void {
	const { runtime, actions, event, itemId } = options;
	const { documents, ui, uiWriter, ghostWriter, snap } = runtime;
	const doc = documents.getState().document;
	const baseRevision = documents.getState().revision;
	if (
		doc === null ||
		baseRevision === null ||
		documents.getState().state === "trashed"
	) {
		return;
	}
	const selected = ui.getState().selection.items;
	const ids = selected.includes(itemId) ? selected : [itemId];
	const dragged: Dragged[] = ids.flatMap((id) => {
		const item = doc.items[id];
		return item === undefined
			? []
			: [
					{
						id,
						start: item.start,
						duration: item.duration,
						track: item.track,
					},
				];
	});
	const grabbed = dragged.find((d) => d.id === itemId);
	if (grabbed === undefined) return;
	const exclude = new Set(dragged.map((d) => d.id));
	const involvesCaption = dragged.some(
		(d) => doc.items[d.id]?.type === "caption",
	);
	const earliest = Math.min(...dragged.map((d) => d.start));

	let ghosts: Record<string, Ghost> = {};
	let targets: MoveTarget[] = [];

	let stopWatching: () => void = () => {};
	const running = runGesture({
		event,
		target: event.currentTarget,
		scroller: event.currentTarget.closest<HTMLElement>(
			"[data-timeline-scroller]",
		),
		gesture: {
			start() {
				if (!selected.includes(itemId)) uiWriter.select([itemId]);
				void snap.begin(
					ui.getState().timeline.pxPerFrame >= WORDS_FROM_PPF &&
						involvesCaption,
				);
			},
			move(sample) {
				const { pxPerFrame: ppf, snapEnabled } = ui.getState().timeline;
				// Never before frame 0: the group stops at the first item's edge.
				let delta = Math.max(deltaPxToFrames(sample.dx, ppf), -earliest);
				const wantStart = grabbed.start + delta;
				const wantEnd = wantStart + grabbed.duration;
				const common = {
					ppf,
					playhead: ui.getState().playheadFrame,
					points: snap.points(),
					exclude,
					enabled: snapEnabled,
					suspended: sample.alt || sample.mod,
				};
				const a = snapFrame({ ...common, frame: wantStart });
				const b = snapFrame({ ...common, frame: wantEnd });
				let line: number | null = null;
				if (
					a.line !== null &&
					(b.line === null ||
						Math.abs(a.frame - wantStart) <= Math.abs(b.frame - wantEnd))
				) {
					delta += a.frame - wantStart;
					line = a.line;
				} else if (b.line !== null) {
					delta += b.frame - wantEnd;
					line = b.line;
				}
				delta = Math.max(delta, -earliest);

				// A single item may change track; a group moves along its tracks.
				const hovered =
					dragged.length === 1
						? trackUnder(sample.clientX, sample.clientY)
						: null;
				ghosts = Object.fromEntries(
					dragged.map((d) => [
						d.id,
						{
							start: d.start + delta,
							track: hovered ?? d.track,
							duration: d.duration,
							pending: false,
						},
					]),
				);
				targets = dragged.map((d) => ({
					id: d.id,
					from: d.start,
					to: d.start + delta,
					trackId:
						hovered !== null && hovered !== d.track ? hovered : undefined,
				}));
				ghostWriter.setGhosts(ghosts);
				ghostWriter.setSnapLine(line);
			},
			async end() {
				const ops = moveItems(targets);
				if (ops.length === 0) {
					ghostWriter.clear();
					return;
				}
				ghostWriter.setSnapLine(null);
				ghostWriter.markPending();
				const outcome = await actions.commitGesture(ops, baseRevision);
				// Answered or refused, the ghost goes: the item is where the read
				// model says it is.
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
		onFinish: () => stopWatching(),
	});
	stopWatching = watchRemoteEdit(
		runtime,
		dragged.map((d) => d.id),
		(actor) => {
			// A press that has not started dragging yet is still the user's, and
			// the core will judge its batch against the revision of the press.
			if (!running.dragging()) return;
			running.cancel();
			runtime.noticeWriter.push({
				kind: "info",
				message: "",
				messageKey: "ui.editor.notice.gesture_cancelled",
				actor,
			});
		},
	);
}

// The item that blocked a move: a static outline for two seconds, scrolled into
// view (SDD-012 section 8.4).
export function outlineConflict(runtime: EditorRuntime, itemId: string): void {
	runtime.ghostWriter.setErrorItem(itemId);
	revealItem(itemId);
	setTimeout(() => {
		if (runtime.ghosts.getState().errorItem === itemId) {
			runtime.ghostWriter.setErrorItem(null);
		}
	}, CONFLICT_OUTLINE_MS);
}
