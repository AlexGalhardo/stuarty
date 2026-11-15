import type { EditOutcome } from "../api/edit";
import type { ApiError } from "../api/errors";
import type { Notice } from "../state/noticeStore";
import type { EditorRuntime } from "../state/runtime";
import {
	selectFps,
	selectOrderedTrackIds,
	selectTrack,
} from "../state/selectors";
import { formatTimecode } from "../ui/timecode";
import {
	addClipOperations,
	adjacentTrack,
	captionOperations,
	type ItemRange,
	moveToTrackOperations,
	nudgeOperations,
	type Operations,
	splitOperations,
	trimOperations,
	withNextFree,
} from "./builders";
import { newId } from "./ids";
import { fpsOf } from "./units";

// The action registry of the Prototype (SDD-012 sections 8.2, 9 and 15): every
// gesture builds its operations from what the user chose and sends one batch
// through the edit queue. None of them touches the read model; it changes when
// the answer or the event arrives (rule T2).
export interface Actions {
	addToTimeline(assetId: string): Promise<EditOutcome | null>;
	splitAtPlayhead(): Promise<EditOutcome | null>;
	trimToPlayhead(edge: "start" | "end"): Promise<EditOutcome | null>;
	addCaption(text: string, duration: string): Promise<EditOutcome | null>;
	// A released gesture: the batch is sent against the revision the pointer went
	// down on (SDD-012 section 4.5), not the one at release.
	commitGesture(
		ops: Operations,
		baseRevision: number,
	): Promise<EditOutcome | null>;
	// Alt+Left and Alt+Right: every item of `ids` by `frames` frames, one batch.
	nudge(ids: readonly string[], frames: number): Promise<EditOutcome | null>;
	// Alt+Up and Alt+Down: every item of `ids` to the track above or below its own.
	moveToAdjacentTrack(
		ids: readonly string[],
		sign: 1 | -1,
	): Promise<EditOutcome | null>;
	// palette.run: the operations the palette built from a schema form.
	runOperations(ops: Operations): Promise<EditOutcome | null>;
	// An inspector field: like runOperations, but a validation or time error is
	// left to the field, which shows it under itself (SDD-012 section 8.4).
	commitField(ops: Operations): Promise<EditOutcome | null>;
	undo(): Promise<EditOutcome>;
	redo(): Promise<EditOutcome>;
	// The action of a notice, as a new logical edit on the current revision.
	applyNotice(notice: Notice): Promise<EditOutcome | null>;
}

// The codes an inspector field shows under itself instead of in a notice.
export const FIELD_ERRORS: ReadonlySet<string> = new Set([
	"E_VALIDATION",
	"E_INVALID_TIME",
]);

// The longest length the source allows, in frames, from `details.available` of
// a trim of the end edge (SDD-002 section 5, R9).
function sourceAvailable(error: ApiError): number | null {
	const available = error.details?.available;
	if (typeof available !== "object" || available === null) return null;
	const max = (available as { [key: string]: unknown }).max_duration;
	return typeof max === "number" ? max : null;
}

function nextFree(error: ApiError): number | null {
	const value = error.details?.next_free;
	return typeof value === "number" ? value : null;
}

export function createActions(runtime: EditorRuntime): Actions {
	const { documents, ui, noticeWriter, uiWriter } = runtime;

	function firstVisualTrack(): string | null {
		const state = documents.getState();
		for (const id of selectOrderedTrackIds(state)) {
			if (selectTrack(state, id)?.kind === "visual") return id;
		}
		return null;
	}

	function selectedRanges(): ItemRange[] {
		return rangesOf(ui.getState().selection.items);
	}

	function rangesOf(ids: readonly string[]): ItemRange[] {
		const doc = documents.getState().document;
		if (doc === null) return [];
		const ranges: ItemRange[] = [];
		for (const id of ids) {
			const item = doc.items[id];
			if (item !== undefined) {
				ranges.push({ id, start: item.start, duration: item.duration });
			}
		}
		return ranges;
	}

	function report(error: ApiError, ops: Operations): void {
		// A request the browser itself gave up is not a failure to show.
		if (error.isAborted) return;
		const base: Omit<Notice, "id"> = {
			kind: "error",
			code: error.code,
			message: error.message,
			hint: error.hint,
		};
		if (error.code === "E_REVISION_CONFLICT") {
			noticeWriter.push({
				...base,
				action: { labelKey: "ui.editor.notice.apply_again", operations: ops },
			});
			return;
		}
		const trimmed = ops.find((o) => o.op === "item.trim");
		const available = sourceAvailable(error);
		if (error.code === "E_OUT_OF_SOURCE" && trimmed && available !== null) {
			const fps = fpsOf(selectFps(documents.getState()));
			noticeWriter.push({
				...base,
				messageKey: "ui.editor.notice.source_has_only",
				params: { available: formatTimecode(available, fps) },
			});
			return;
		}
		const free = error.code === "E_OVERLAP" ? nextFree(error) : null;
		if (
			free !== null &&
			ops.some(
				(o) =>
					o.op.endsWith(".add") || o.op === "item.move" || o.op === "item.trim",
			)
		) {
			noticeWriter.push({
				...base,
				action: {
					labelKey: "ui.editor.notice.next_free",
					operations: withNextFree(ops, free),
				},
			});
			return;
		}
		noticeWriter.push(base);
	}

	// One gesture, one request. The base revision is the one at the moment of
	// sending (SDD-012 section 4.5 item 1).
	async function send(
		ops: Operations,
		inline = false,
		baseRevision?: number,
	): Promise<EditOutcome | null> {
		const { revision, state } = documents.getState();
		// A trashed project is read-only (SDD-012 section 2.2): the keys too.
		if (ops.length === 0 || revision === null || state === "trashed")
			return null;
		const outcome = await runtime.edits.edit(ops, baseRevision ?? revision);
		if (!outcome.ok && !(inline && FIELD_ERRORS.has(outcome.error.code)))
			report(outcome.error, ops);
		return outcome;
	}

	async function history(kind: "undo" | "redo"): Promise<EditOutcome> {
		const outcome = await (kind === "undo"
			? runtime.edits.undo()
			: runtime.edits.redo());
		if (outcome.ok) {
			const skippedOps = outcome.result.skipped ?? [];
			const skipped = skippedOps.length;
			if (skipped > 0) {
				noticeWriter.push({
					kind: "info",
					message: "",
					messageKey: `ui.editor.notice.${kind}_skipped`,
					count: skipped,
					actor: skippedOps.find((op) => op.by_actor !== undefined)?.by_actor,
				});
			}
			return outcome;
		}
		if (outcome.error.isAborted) return outcome;
		const code = outcome.error.code;
		if (code === "E_NOTHING_TO_UNDO" || code === "E_NOTHING_TO_REDO") {
			uiWriter.blockHistory(kind, documents.getState().revision ?? 0);
		} else {
			noticeWriter.push({
				kind: "error",
				code,
				message: outcome.error.message,
				hint: outcome.error.hint,
			});
		}
		return outcome;
	}

	return {
		addToTimeline: (assetId) => {
			const state = documents.getState();
			// Placement is offered only for an asset the read model has: the
			// asset.updated event and the asset.add batch are not ordered.
			if (state.document?.assets[assetId] === undefined) {
				return Promise.resolve(null);
			}
			return send(
				addClipOperations({
					assetId,
					playhead: ui.getState().playheadFrame,
					firstVisualTrack: firstVisualTrack(),
					newTrackId: () => newId("trk"),
				}),
			);
		},
		splitAtPlayhead: () =>
			send(splitOperations(selectedRanges(), ui.getState().playheadFrame)),
		trimToPlayhead: (edge) =>
			send(trimOperations(selectedRanges(), edge, ui.getState().playheadFrame)),
		addCaption: (text, duration) =>
			send(
				captionOperations({
					text,
					duration,
					playhead: ui.getState().playheadFrame,
					firstVisualTrack: firstVisualTrack(),
					newTrackId: () => newId("trk"),
				}),
			),
		nudge: (ids, frames) => send(nudgeOperations(rangesOf(ids), frames)),
		moveToAdjacentTrack(ids, sign) {
			const doc = documents.getState().document;
			if (doc === null) return Promise.resolve(null);
			const tracks = selectOrderedTrackIds(documents.getState());
			const moves = ids.flatMap((itemId) => {
				const item = doc.items[itemId];
				const trackId =
					item === undefined ? null : adjacentTrack(tracks, item.track, sign);
				return trackId === null ? [] : [{ itemId, trackId }];
			});
			return send(moveToTrackOperations(moves));
		},
		commitGesture: (ops, baseRevision) => send(ops, false, baseRevision),
		runOperations: (ops) => send(ops),
		commitField: (ops) => send(ops, true),
		undo: () => history("undo"),
		redo: () => history("redo"),
		async applyNotice(notice) {
			noticeWriter.dismiss(notice.id);
			if (notice.action === undefined) return null;
			return send(notice.action.operations);
		},
	};
}
