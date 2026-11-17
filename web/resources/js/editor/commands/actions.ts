import { announce } from "../a11y/announce";
import type { EditOutcome } from "../api/edit";
import type { ApiError } from "../api/errors";
import type { Notice } from "../state/noticeStore";
import { readTimeDisplay } from "../state/prefs";
import type { EditorRuntime } from "../state/runtime";
import {
	selectFps,
	selectOrderedTrackIds,
	selectTrack,
	selectTrackItemIds,
} from "../state/selectors";
import {
	type AnyKeyframeProperty,
	addAudioOperations,
	addClipOperations,
	adjacentTrack,
	CROSSFADE_DURATION,
	captionEditWordOperations,
	captionMergeOperations,
	captionOperations,
	captionRetimeOperations,
	captionSplitOperations,
	closeGapOperations,
	crossfadeOperations,
	deleteOperations,
	duplicateOperations,
	enabledOperations,
	extractAudioOperations,
	freezeFrameOperations,
	type ItemRange,
	keyframeFallbackOperations,
	keyframeFrame,
	keyframeRemoveOperations,
	linkOperations,
	markerAddOperations,
	markerRemoveOperations,
	moveToTrackOperations,
	nudgeOperations,
	type Operations,
	pasteAttributesOperations,
	pasteOperations,
	projectRenameOperations,
	type Ripple,
	removeAssetOperations,
	renameOperations,
	rollOperations,
	type ShapeKind,
	settingsFpsOperations,
	shapeAddOperations,
	slideOperations,
	slipOperations,
	splitOperations,
	type TrackChanges,
	type TrackPlacement,
	textAddOperations,
	textSetOperations,
	trackAddOperations,
	trackMoveOperations,
	trackRemoveOperations,
	trackUpdateOperations,
	trimOperations,
	unlinkOperations,
	volumeStepOperations,
	withNextFree,
} from "./builders";
import { newId } from "./ids";
import { magnetTrackId } from "./magnet";
import { formatTime, fpsOf } from "./units";

// The action registry of the Prototype (SDD-012 sections 8.2, 9 and 15): every
// gesture builds its operations from what the user chose and sends one batch
// through the edit queue. None of them touches the read model; it changes when
// the answer or the event arrives (rule T2).
export interface Actions {
	// `track` chooses the destination: an existing track id, or null for a new
	// track added in the same batch; left out, it is the track of the selected
	// item, else the first track of the kind (VV-092). `start` is never sent.
	addToTimeline(
		assetId: string,
		track?: string | null,
	): Promise<EditOutcome | null>;
	splitAtPlayhead(): Promise<EditOutcome | null>;
	// The blade tool: one item cut at the frame under the pointer.
	splitItemAt(itemId: string, frame: number): Promise<EditOutcome | null>;
	// M: a marker at the playhead, or removes the one that is already there.
	toggleMarkerAtPlayhead(): Promise<EditOutcome | null>;
	// Mod+Shift+B: every item under the playhead, on every track.
	splitAllAtPlayhead(): Promise<EditOutcome | null>;
	trimToPlayhead(edge: "start" | "end"): Promise<EditOutcome | null>;
	addCaption(text: string, duration: string): Promise<EditOutcome | null>;
	// Toolbar "Add text" and `T`: `text` is the localized default content.
	addText(text: string): Promise<EditOutcome | null>;
	// Toolbar "Add shape" menu.
	addShape(shape: ShapeKind): Promise<EditOutcome | null>;
	// Double click on a caption word; empty text removes the word.
	editWord(
		itemId: string,
		index: number,
		text: string,
	): Promise<EditOutcome | null>;
	// "Split caption here": the word at `beforeWord` starts the right part.
	splitCaption(itemId: string, beforeWord: number): Promise<EditOutcome | null>;
	// "Merge" with two captions selected.
	mergeCaptions(itemIds: readonly string[]): Promise<EditOutcome | null>;
	// "Generate" of the auto captions panel: the batch of
	// `captionFromTranscriptOperations`, sent once the transcript exists.
	generateCaptions(ops: Operations): Promise<EditOutcome | null>;
	// "Re-sync captions" of a clip, in the panel and in the lint finding.
	resyncCaptions(clipId: string): Promise<EditOutcome | null>;
	// The inline editor of the canvas and the timeline: only a change of the
	// content is sent, and the error is a notice, not a field message.
	setText(itemId: string, text: string): Promise<EditOutcome | null>;
	// A released gesture: the batch is sent against the revision the pointer went
	// down on (SDD-012 section 4.5), not the one at release.
	commitGesture(
		ops: Operations,
		baseRevision: number,
	): Promise<EditOutcome | null>;
	// Alt+Left and Alt+Right: every item of `ids` by `frames` frames, one batch.
	nudge(ids: readonly string[], frames: number): Promise<EditOutcome | null>;
	// Mod+Alt+Left and Right: slide, Mod+Alt+Shift+Left and Right: roll the cut
	// after the item, Alt+, and Alt+.: slip. Every item of `ids` by `frames`.
	slideItems(
		ids: readonly string[],
		frames: number,
	): Promise<EditOutcome | null>;
	rollItems(
		ids: readonly string[],
		frames: number,
	): Promise<EditOutcome | null>;
	slipItems(
		ids: readonly string[],
		frames: number,
	): Promise<EditOutcome | null>;
	// Alt+Up and Alt+Down: every item of `ids` to the track above or below its own.
	moveToAdjacentTrack(
		ids: readonly string[],
		sign: 1 | -1,
	): Promise<EditOutcome | null>;
	// Delete, Shift+Delete and the menu: the selected keyframe, else the
	// selection, else the focused item.
	deleteSelection(ripple?: Ripple): Promise<EditOutcome | null>;
	// The menu entry "Crossfade" with exactly two items selected.
	crossfadeSelection(): Promise<EditOutcome | null>;
	// Ctrl+D: a copy of each selected item right after its end.
	duplicateSelection(): Promise<EditOutcome | null>;
	// V and the menu: every selected item off when any is on, else all on.
	toggleEnabled(): Promise<EditOutcome | null>;
	// `~` and the menu: the selected items are unlinked when any is in a link
	// group, else linked into one.
	toggleLink(): Promise<EditOutcome | null>;
	linkSelection(): Promise<EditOutcome | null>;
	unlinkSelection(): Promise<EditOutcome | null>;
	// Mod+Shift+S and the menu: the audio of each selected clip goes to an audio
	// track as a linked item and the clip is muted (`item.extract_audio`).
	extractAudio(): Promise<EditOutcome | null>;
	// P: turns the magnet of a track on or off (`track.update` with `magnet`). The
	// track is the selected one, else the track of the selected or focused item,
	// else the first visual track (the main one); the new value is the opposite
	// of what the core stores for it.
	toggleMagnet(): Promise<EditOutcome | null>;
	// Holds the picture at the playhead of the first selected item the playhead
	// lies inside (`item.freeze_frame`); the core judges the kind of item.
	freezeFrame(): Promise<EditOutcome | null>;
	// Mod+Shift+C: remembers the first selected item as the source of the look
	// to paste; false when nothing is selected.
	copyAttributes(): boolean;
	// Mod+Shift+V: sends `item.paste_attributes` from the remembered item to
	// the other selected ones; the core skips the groups a target lacks.
	pasteAttributes(): Promise<EditOutcome | null>;
	// Paste (Mod+V): the clipboard items at the playhead frame `at`; a cut
	// clipboard moves them, a copy duplicates them (`pasteOperations`).
	pasteItems(
		ids: readonly string[],
		at: number,
		cut: boolean,
	): Promise<EditOutcome | null>;
	// Mod+. and Mod+,: the volume of the selected clips and audio items, moved
	// by `db` decibels (one `item.set_audio` each, one batch).
	adjustVolume(db: number): Promise<EditOutcome | null>;
	// The gap menu: `at` is the frame under the pointer.
	closeGap(trackId: string, at: number): Promise<EditOutcome | null>;
	// "Remove from project" on a card; E_IN_USE opens the dialog of the items.
	removeAsset(assetId: string): Promise<EditOutcome | null>;
	// The inline rename (F2, the menu); an empty name clears it.
	renameItem(itemId: string, name: string): Promise<EditOutcome | null>;
	// The track header menu "Add track" above or below `anchorId`.
	addTrack(
		kind: "visual" | "audio",
		placement: TrackPlacement,
	): Promise<EditOutcome | null>;
	// Rename, hide and mute of a track header and of the track panel.
	updateTrack(
		trackId: string,
		changes: TrackChanges,
	): Promise<EditOutcome | null>;
	// The reorder drag and "Move up" and "Move down".
	moveTrack(
		trackId: string,
		placement: TrackPlacement,
	): Promise<EditOutcome | null>;
	// `E_NOT_EMPTY` is not a notice: the caller asks, then sends it again with
	// `withItems` (SDD-012 section 4.4).
	removeTrack(trackId: string, withItems: boolean): Promise<EditOutcome | null>;
	// The top bar name: Enter or blur sends `project.rename`.
	renameProject(name: string): Promise<EditOutcome | null>;
	// The frame rate select, after the confirmation: the answer carries
	// `effective.adjusted`, which the dialog lists.
	setFps(fps: string): Promise<EditOutcome | null>;
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
	"E_FONT_UNKNOWN",
]);

// The longest length the source allows, in frames, from `details.available` of
// a trim of the end edge (SDD-002 section 5, R9).
function sourceAvailable(error: ApiError): number | null {
	const available = error.details?.available;
	if (typeof available !== "object" || available === null) return null;
	const max = (available as { [key: string]: unknown }).max_duration;
	return typeof max === "number" ? max : null;
}

const CATCH_UP_MS = 2000;

// The facts of an E_REVISION_CONFLICT, as the core sent them (SDD-006 section 7.3).
function conflictOf(error: ApiError): NonNullable<Notice["conflict"]> | null {
	const details = error.details;
	const current = details?.current_revision;
	const conflict = details?.conflict;
	if (typeof current !== "number") return null;
	const who =
		typeof conflict === "object" && conflict !== null
			? (conflict as { [key: string]: unknown })
			: {};
	return {
		actor: typeof who.actor === "string" ? who.actor : "",
		itemId:
			who.collection === "items" && typeof who.id === "string" ? who.id : null,
		diff: typeof details?.diff === "string" ? details.diff : "",
		revision: current,
	};
}

function nextFree(error: ApiError): number | null {
	const value = error.details?.next_free;
	return typeof value === "number" ? value : null;
}

// The item whose look was copied, one per editor runtime: the menu and the keys each build their own
// actions, so a variable inside `createActions` would be forgotten between "Copy attributes" and
// "Paste attributes" (VV-091).
const attributeSources = new WeakMap<EditorRuntime, string>();

export function createActions(runtime: EditorRuntime): Actions {
	const { documents, ui, noticeWriter, uiWriter } = runtime;

	function firstTrackOfKind(kind: "visual" | "audio"): string | null {
		const state = documents.getState();
		for (const id of selectOrderedTrackIds(state)) {
			if (selectTrack(state, id)?.kind === kind) return id;
		}
		return null;
	}

	// Where "Add to timeline" puts a clip: the track of the selected item when it is of the right kind,
	// else the first one (VV-092).
	function destinationTrack(kind: "visual" | "audio"): string | null {
		const state = documents.getState();
		for (const id of ui.getState().selection.items) {
			const track = state.document?.items[id]?.track;
			if (track !== undefined && selectTrack(state, track)?.kind === kind) {
				return track;
			}
		}
		return firstTrackOfKind(kind);
	}

	function selectedRanges(): ItemRange[] {
		return rangesOf(ui.getState().selection.items);
	}

	// The items a key or a menu entry acts on: the selection, else the item that
	// holds the keyboard focus.
	function actedIds(): string[] {
		const { selection, focusedItem } = ui.getState();
		if (selection.items.length > 0) return [...selection.items];
		return focusedItem === null ? [] : [focusedItem];
	}

	function rangesOf(ids: readonly string[]): ItemRange[] {
		const doc = documents.getState().document;
		if (doc === null) return [];
		const ranges: ItemRange[] = [];
		for (const id of ids) {
			const item = doc.items[id];
			if (item !== undefined) {
				ranges.push({
					id,
					start: item.start,
					duration: item.duration,
					track: item.track,
				});
			}
		}
		return ranges;
	}

	// An empty visual track other than the shared one: nothing can be in the way
	// of the arriving item there. No overlap arithmetic in the client; with none
	// empty the batch adds a track, and the core judges the move.
	function emptyVisualTrackBut(shared: string | undefined): string | null {
		const state = documents.getState();
		for (const id of selectOrderedTrackIds(state)) {
			if (id === shared || selectTrack(state, id)?.kind !== "visual") continue;
			if (selectTrackItemIds(state, id).length === 0) return id;
		}
		return null;
	}

	// Wait for the read model to reach `revision`: the events normally are there
	// already; after 2 s the editor resyncs (SDD-012 section 4.5 item 2).
	function catchUp(revision: number): Promise<void> {
		const reached = (): boolean =>
			(documents.getState().revision ?? 0) >= revision;
		if (reached()) return Promise.resolve();
		return new Promise((resolve) => {
			const stop = documents.subscribe(() => {
				if (!reached()) return;
				stop();
				clearTimeout(timer);
				resolve();
			});
			const timer = setTimeout(() => {
				stop();
				void runtime.sync.resync().finally(resolve);
			}, CATCH_UP_MS);
		});
	}

	// The result of a key or a menu entry, read out by the live region
	// (SDD-012 section 18.1 item 5). Field edits of the inspector are not.
	function announceEdit(ops: Operations): void {
		const count = (...names: string[]): number =>
			ops.filter((o) => names.includes(o.op)).length;
		const splits = count("item.split", "caption.split");
		const moves = count("item.move");
		const deletes = count("item.delete");
		if (splits > 0) announce("ui.editor.announce.split", { count: splits });
		else if (deletes > 0)
			announce("ui.editor.announce.deleted", { count: deletes });
		else if (moves > 0) announce("ui.editor.announce.moved", { count: moves });
	}

	function report(error: ApiError, ops: Operations): void {
		// A request the browser itself gave up is not a failure to show.
		if (error.isAborted) return;
		const base: Omit<Notice, "id"> = {
			kind: "error",
			code: error.code,
			message: error.message,
			hint: error.hint,
			details: error.details,
		};
		if (error.code === "E_REVISION_CONFLICT") {
			const found = conflictOf(error);
			noticeWriter.push({
				...base,
				conflict: found === null ? undefined : found,
				action: { labelKey: "ui.editor.notice.apply_again", operations: ops },
			});
			// Bring the read model to the revision the core named; the UI never
			// resends the batch on its own (SDD-012 section 4.5 item 4).
			if (found !== null) void catchUp(found.revision);
			return;
		}
		// The track removal dialog asks for the confirmation itself.
		if (
			error.code === "E_NOT_EMPTY" &&
			ops.some((o) => o.op === "track.remove")
		) {
			return;
		}
		if (error.code === "E_IN_USE") {
			const usedBy = error.details?.used_by;
			if (Array.isArray(usedBy)) {
				const removed = ops.find((o) => o.op === "asset.remove")?.args.asset_id;
				uiWriter.showInUse({
					assetId: typeof removed === "string" ? removed : "",
					usedBy: usedBy.filter((id): id is string => typeof id === "string"),
				});
				return;
			}
		}
		const trimmed = ops.find((o) => o.op === "item.trim");
		const available = sourceAvailable(error);
		if (error.code === "E_OUT_OF_SOURCE" && trimmed && available !== null) {
			const fps = fpsOf(selectFps(documents.getState()));
			noticeWriter.push({
				...base,
				messageKey: "ui.editor.notice.source_has_only",
				params: { available: formatTime(available, fps, readTimeDisplay()) },
			});
			return;
		}
		const free = error.code === "E_OVERLAP" ? nextFree(error) : null;
		const failedOp = ops[error.opIndex ?? 0];
		if (
			free !== null &&
			failedOp !== undefined &&
			(failedOp.op.endsWith(".add") ||
				failedOp.op === "item.move" ||
				failedOp.op === "item.trim")
		) {
			noticeWriter.push({
				...base,
				action: {
					labelKey: "ui.editor.notice.next_free",
					operations: withNextFree(ops, free, error.opIndex ?? 0),
				},
			});
			return;
		}
		if (
			error.code === "E_INVALID_STATE" &&
			ops.some((o) => o.op === "item.set_transform")
		) {
			noticeWriter.push({
				...base,
				action: {
					labelKey: "ui.editor.notice.use_keyframes",
					operations: keyframeFallbackOperations(
						ops,
						ui.getState().playheadFrame,
					),
				},
			});
			return;
		}
		const failed = ops[error.opIndex ?? 0];
		const splitId =
			failed?.op === "caption.split" || failed?.op === "item.split"
				? failed.args.item_id
				: undefined;
		if (
			error.code === "E_INVALID_STATE" &&
			typeof splitId === "string" &&
			documents.getState().document?.items[splitId]?.type === "caption"
		) {
			// Split at a word boundary: the button selects the caption, whose
			// panel lists the words (SDD-012 section 8.4).
			noticeWriter.push({
				...base,
				action: { labelKey: "ui.editor.notice.open_words", select: splitId },
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
		if (outcome.ok) announceEdit(ops);
		else if (!(inline && FIELD_ERRORS.has(outcome.error.code))) {
			report(outcome.error, ops);
			if (!outcome.error.isAborted)
				announce(`ui.editor.errors.${outcome.error.code}`);
		}
		return outcome;
	}

	async function history(kind: "undo" | "redo"): Promise<EditOutcome> {
		const outcome = await (kind === "undo"
			? runtime.edits.undo()
			: runtime.edits.redo());
		if (outcome.ok) {
			announce(`ui.editor.announce.${kind === "undo" ? "undone" : "redone"}`);
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
		addToTimeline: (assetId, track) => {
			const state = documents.getState();
			// Placement is offered only for an asset the read model has: the
			// asset.updated event and the asset.add batch are not ordered.
			if (state.document?.assets[assetId] === undefined) {
				return Promise.resolve(null);
			}
			if (state.document.assets[assetId]?.kind === "audio") {
				return send(
					addAudioOperations({
						assetId,
						firstAudioTrack:
							track === undefined ? destinationTrack("audio") : track,
						newTrackId: () => newId("trk"),
					}),
				);
			}
			return send(
				addClipOperations({
					assetId,
					firstVisualTrack:
						track === undefined ? destinationTrack("visual") : track,
					newTrackId: () => newId("trk"),
				}),
			);
		},
		toggleMarkerAtPlayhead() {
			const doc = documents.getState().document;
			if (doc === null) return Promise.resolve(null);
			const frame = ui.getState().playheadFrame;
			const there = (doc.markers ?? []).some((m) => m.frame === frame);
			return send(
				there ? markerRemoveOperations(frame) : markerAddOperations(frame),
			);
		},
		splitAtPlayhead: () =>
			send(splitOperations(selectedRanges(), ui.getState().playheadFrame)),
		splitItemAt: (itemId, frame) =>
			send(splitOperations(rangesOf([itemId]), frame)),
		splitAllAtPlayhead() {
			const doc = documents.getState().document;
			if (doc === null) return Promise.resolve(null);
			return send(
				splitOperations(
					rangesOf(Object.keys(doc.items)),
					ui.getState().playheadFrame,
				),
			);
		},
		// Q and W, and every other trim-to-playhead control, close the gap. The
		// core default of item.trim stays none for agents (SDD-012 §6.5).
		trimToPlayhead: (edge) =>
			send(
				trimOperations(
					selectedRanges(),
					edge,
					ui.getState().playheadFrame,
					"track",
				),
			),
		addCaption: (text, duration) =>
			send(
				captionOperations({
					text,
					duration,
					playhead: ui.getState().playheadFrame,
				}),
			),
		addText: (text) =>
			send(
				textAddOperations({
					text,
					playhead: ui.getState().playheadFrame,
				}),
			),
		addShape: (shape) =>
			send(
				shapeAddOperations({
					shape,
					playhead: ui.getState().playheadFrame,
				}),
			),
		editWord: (itemId, index, text) =>
			send(captionEditWordOperations(itemId, index, text)),
		splitCaption: (itemId, beforeWord) =>
			send(captionSplitOperations(itemId, beforeWord)),
		mergeCaptions: (itemIds) => send(captionMergeOperations(itemIds)),
		generateCaptions: (ops) => send(ops),
		resyncCaptions: (clipId) => send(captionRetimeOperations(clipId)),
		setText: (itemId, text) => send(textSetOperations(itemId, text)),
		nudge: (ids, frames) => send(nudgeOperations(rangesOf(ids), frames)),
		slideItems: (ids, frames) => send(slideOperations(rangesOf(ids), frames)),
		rollItems: (ids, frames) => send(rollOperations(rangesOf(ids), frames)),
		slipItems(ids, frames) {
			const doc = documents.getState().document;
			if (doc === null) return Promise.resolve(null);
			const targets = ids.flatMap((id) => {
				const item = doc.items[id];
				return item?.type === "clip" || item?.type === "audio"
					? [{ id, sourceIn: item.source_in ?? 0 }]
					: [];
			});
			return send(slipOperations(targets, frames));
		},
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
		deleteSelection(ripple) {
			// A selected keyframe is what Delete removes, before any item.
			const keyframe = ui.getState().selection.keyframe;
			const item =
				keyframe === null
					? undefined
					: documents.getState().document?.items[keyframe.item];
			if (keyframe !== null && item !== undefined) {
				return send(
					keyframeRemoveOperations(
						keyframe.item,
						keyframe.property as AnyKeyframeProperty,
						keyframeFrame(item.start, keyframe.t),
					),
				);
			}
			return send(deleteOperations(actedIds(), ripple));
		},
		duplicateSelection: () => send(duplicateOperations(rangesOf(actedIds()))),
		toggleEnabled() {
			const doc = documents.getState().document;
			if (doc === null) return Promise.resolve(null);
			const items = actedIds().flatMap((id) => {
				const item = doc.items[id];
				return item === undefined ? [] : [{ id, off: item.disabled === true }];
			});
			const enable = items.length > 0 && items.every((item) => item.off);
			return send(items.flatMap((item) => enabledOperations(item.id, enable)));
		},
		linkSelection: () => send(linkOperations(actedIds())),
		unlinkSelection: () => send(unlinkOperations(actedIds())),
		toggleLink() {
			const doc = documents.getState().document;
			if (doc === null) return Promise.resolve(null);
			const ids = actedIds();
			const linked = ids.filter((id) => doc.items[id]?.link !== undefined);
			if (linked.length > 0) return send(unlinkOperations(linked));
			// The core links two to twenty items; one item is told so, not sent (VV-125).
			if (ids.length < 2) {
				noticeWriter.push({
					kind: "info",
					message: "",
					messageKey: "ui.editor.notice.link_needs_two",
				});
				return Promise.resolve(null);
			}
			return send(linkOperations(ids));
		},
		extractAudio() {
			const doc = documents.getState().document;
			if (doc === null) return Promise.resolve(null);
			const clips = actedIds().filter((id) => doc.items[id]?.type === "clip");
			const [first, ...rest] = clips;
			if (first === undefined) return Promise.resolve(null);
			// One audio track for the batch: an empty one, else one added first.
			const state = documents.getState();
			const track =
				selectOrderedTrackIds(state).find(
					(id) =>
						selectTrack(state, id)?.kind === "audio" &&
						selectTrackItemIds(state, id).length === 0,
				) ?? null;
			const newTrack = newId("trk");
			const ops = extractAudioOperations({
				itemId: first,
				audioTrack: track,
				newTrackId: () => newTrack,
			});
			for (const id of rest) {
				ops.push(
					...extractAudioOperations({
						itemId: id,
						audioTrack: track ?? newTrack,
						newTrackId: () => newTrack,
					}).filter((op) => op.op === "item.extract_audio"),
				);
			}
			return send(ops);
		},
		toggleMagnet() {
			const state = documents.getState();
			const doc = state.document;
			if (doc === null) return Promise.resolve(null);
			const trackId = magnetTrackId(state, ui.getState());
			const track = trackId === null ? undefined : doc.tracks[trackId];
			if (trackId === null || track === undefined) return Promise.resolve(null);
			return send(
				trackUpdateOperations(trackId, { magnet: track.magnet !== true }),
			);
		},
		freezeFrame: () =>
			send(
				freezeFrameOperations(selectedRanges(), ui.getState().playheadFrame),
			),
		copyAttributes() {
			const [first] = actedIds();
			if (first === undefined) return false;
			attributeSources.set(runtime, first);
			return true;
		},
		pasteAttributes() {
			const from = attributeSources.get(runtime);
			if (from === undefined) return Promise.resolve(null);
			return send(
				pasteAttributesOperations(
					from,
					actedIds().filter((id) => id !== from),
				),
			);
		},
		pasteItems: (ids, at, cut) => send(pasteOperations(rangesOf(ids), at, cut)),
		adjustVolume(db) {
			const doc = documents.getState().document;
			if (doc === null) return Promise.resolve(null);
			const targets = actedIds().flatMap((id) => {
				const item = doc.items[id];
				return item?.type === "clip" || item?.type === "audio"
					? [{ id, volume: item.volume ?? 1 }]
					: [];
			});
			return send(volumeStepOperations(targets, db));
		},
		crossfadeSelection() {
			const [a, b, ...rest] = selectedRanges();
			if (a === undefined || b === undefined || rest.length > 0) {
				return Promise.resolve(null);
			}
			return send(
				crossfadeOperations(a, b, CROSSFADE_DURATION, {
					freeTrack: emptyVisualTrackBut(a.track),
					newTrackId: () => newId("trk"),
				}),
			);
		},
		closeGap: (trackId, at) => send(closeGapOperations(trackId, at)),
		async removeAsset(assetId) {
			const outcome = await send(removeAssetOperations(assetId));
			await runtime.reloadLibrary();
			return outcome;
		},
		renameItem: (itemId, name) => send(renameOperations(itemId, name)),
		addTrack: (kind, placement) => send(trackAddOperations(kind, placement)),
		updateTrack: (trackId, changes) =>
			send(trackUpdateOperations(trackId, changes)),
		moveTrack: (trackId, placement) =>
			send(trackMoveOperations(trackId, placement)),
		removeTrack: (trackId, withItems) =>
			send(trackRemoveOperations(trackId, withItems)),
		renameProject: (name) => send(projectRenameOperations(name)),
		setFps: (fps) => send(settingsFpsOperations(fps)),
		commitGesture: (ops, baseRevision) => send(ops, false, baseRevision),
		runOperations: (ops) => send(ops),
		commitField: (ops) => send(ops, true),
		undo: () => history("undo"),
		redo: () => history("redo"),
		async applyNotice(notice) {
			noticeWriter.dismiss(notice.id);
			if (notice.action === undefined) return null;
			if (notice.action.select !== undefined) {
				uiWriter.select([notice.action.select]);
				return null;
			}
			if (notice.action.operations === undefined) return null;
			// A new logical edit on the current revision, the explicit decision of
			// the user to act on top of the other change (SDD-012 section 4.5 item 3).
			if (notice.conflict !== undefined)
				await catchUp(notice.conflict.revision);
			return send(notice.action.operations);
		},
	};
}
