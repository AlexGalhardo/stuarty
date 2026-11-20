import { createStore, type StoreApi } from "zustand/vanilla";

import { ApiError } from "../../editor/api/errors";
import type { ImageApi } from "../api/imageApi";
import type {
	ImageDocument,
	ImageEditResult,
	ImageHistoryEntry,
	ImageImportResult,
	ImageOperation,
	ImageStreamEvent,
	Layer,
} from "../api/types";
import {
	effectiveZoom,
	type Point,
	type Size,
	zoomAbout,
} from "../canvas/viewport";

// A view of the document in the browser (SDD-012 section 24.2): the last answer of the server, the selection,
// the viewport and what the screen says about the other actors. Nothing here is a result of an edit: every
// change goes out as a batch, and the answer replaces `doc`.

export type LoadState = "loading" | "ready" | "failed";

export interface Viewport {
	// 1 is 100%. `fit` follows the size of the stage until a person zooms.
	zoom: number;
	fit: boolean;
	panX: number;
	panY: number;
}

export interface Notice {
	kind: "agent";
	actor: string;
}

export interface ImageState {
	imageId: string;
	doc: ImageDocument | null;
	load: LoadState;
	error: ApiError | null;
	busy: boolean;
	selection: readonly string[];
	view: Viewport;
	// The size of the stage the canvas is drawn in, reported by the canvas.
	stage: Size;
	history: readonly ImageHistoryEntry[];
	// The layers another actor changed with the latest revision, for the outline.
	changedByOthers: readonly string[];
	notice: Notice | null;
}

// What a command sends: the operations, or a function of the document at the moment its turn comes, for a key
// that repeats faster than the server answers (an arrow, Alt with an arrow): each press is planned against the
// document the one before it made.
export type Operations =
	| readonly ImageOperation[]
	| ((doc: ImageDocument) => readonly ImageOperation[]);

// What a person gives to put a file in this image: the path of a file on the machine of the server, or a file
// chosen in the browser (SDD-021 section 10). The server decides everything about the file.
export type ImportSource =
	| { path: string; name?: string }
	| { file: File; name?: string };

export interface ImageActions {
	open(): Promise<void>;
	// Sends one batch against the revision on screen; resolves with the result, or null when it failed (the
	// failure is in `error`). `select` picks the layers to select from the result.
	send(
		operations: Operations,
		select?: (result: ImageEditResult) => readonly string[] | undefined,
	): Promise<ImageEditResult | null>;
	undo(): Promise<void>;
	redo(): Promise<void>;
	// Puts a file in this image as one more layer (SDD-021 section 10.1); resolves with the result, or null when
	// it failed (the failure is in `error`).
	importFile(source: ImportSource): Promise<ImageImportResult | null>;
	// The polling of the revision: the fallback while the event stream is not open.
	poll(): Promise<void>;
	// An event of the stream of this image (SDD-006 section 12): a revision above the one on screen is fetched, one
	// at or below it (this window's own commit, which the response already applied) is ignored.
	receive(event: ImageStreamEvent): Promise<void>;
	refreshHistory(): Promise<void>;
	select(ids: readonly string[]): void;
	setView(view: Partial<Viewport>): void;
	setStage(stage: Size): void;
	// Zoom by a factor about a point of the stage (the centre when omitted); fit and 100% reset the pan.
	zoomBy(factor: number, at?: Point): void;
	fitView(): void;
	actualSize(): void;
	clearError(): void;
	clearNotice(): void;
}

export type ImageStore = StoreApi<ImageState & ImageActions>;

const INITIAL_VIEW: Viewport = { zoom: 1, fit: true, panX: 0, panY: 0 };

// The ids of the layers of the tree, parents first.
export function layerIds(layers: readonly Layer[]): string[] {
	const ids: string[] = [];
	for (const layer of layers) {
		ids.push(layer.id);
		if (layer.children) ids.push(...layerIds(layer.children));
	}
	return ids;
}

function flat(layers: readonly Layer[]): Map<string, string> {
	const out = new Map<string, string>();
	const visit = (list: readonly Layer[]): void => {
		for (const layer of list) {
			const { children, ...own } = layer;
			out.set(layer.id, JSON.stringify(own));
			if (children) visit(children);
		}
	};
	visit(layers);
	return out;
}

// The layers that differ between two revisions, for the outline of what another actor did. Display only.
export function changedLayers(
	before: readonly Layer[],
	after: readonly Layer[],
): string[] {
	const old = flat(before);
	const out: string[] = [];
	for (const [id, text] of flat(after)) {
		if (old.get(id) !== text) out.push(id);
	}
	return out;
}

function asApiError(error: unknown): ApiError {
	return error instanceof ApiError
		? error
		: new ApiError(0, { code: "E_INTERNAL", message: String(error) });
}

export function createImageStore(api: ImageApi, imageId: string): ImageStore {
	let queue: Promise<unknown> = Promise.resolve();
	// Every call that changes the document runs after the one before it, so each batch is planned against the
	// revision the previous one made.
	const enqueue = <T>(task: () => Promise<T>): Promise<T> => {
		const next = queue.then(task, task);
		queue = next.catch(() => undefined);
		return next;
	};

	// The newest revision an event of the stream named, and whether a catch-up is running or asked for again.
	let seenRevision = 0;
	let catching = false;
	let again = false;

	return createStore<ImageState & ImageActions>()((set, get) => {
		const keepSelection = (doc: ImageDocument): readonly string[] => {
			const ids = new Set(layerIds(doc.layers));
			return get().selection.filter((id) => ids.has(id));
		};

		const replace = (
			doc: ImageDocument,
			extra: Partial<ImageState> = {},
		): void => {
			set({ doc, load: "ready", selection: keepSelection(doc), ...extra });
		};

		// Asks for the document again when it is behind what the server holds, and outlines the layers another
		// actor changed. The reader of an event, and the poll while the stream is down, both come here; calls that
		// arrive while one runs are folded into one more round.
		const catchUp = async (): Promise<void> => {
			if (catching) {
				again = true;
				return;
			}
			catching = true;
			try {
				do {
					again = false;
					if (get().doc === null || get().busy) break;
					try {
						const fresh = await api.describe(imageId);
						const now = get().doc;
						if (now === null || get().busy || fresh.revision <= now.revision)
							continue;
						const changed = changedLayers(now.layers, fresh.layers);
						replace(fresh, { changedByOthers: changed });
						const held = now.revision;
						await get().refreshHistory();
						const other = get().history.find(
							(entry) => entry.revision > held && !entry.mine,
						);
						if (other) set({ notice: { kind: "agent", actor: other.actor } });
					} catch {
						// A catch-up that fails is tried again by the next event or tick; a command shows its own errors.
					}
				} while (again);
			} finally {
				catching = false;
			}
		};

		// After a command of this window: an event of another actor that arrived while it ran was left for now.
		const settle = (): void => {
			const doc = get().doc;
			if (doc !== null && seenRevision > doc.revision) void catchUp();
		};

		const step = (kind: "undo" | "redo"): Promise<void> =>
			enqueue(async () => {
				if (get().doc === null) return;
				set({ busy: true, error: null });
				try {
					await (kind === "undo" ? api.undo(imageId) : api.redo(imageId));
					replace(await api.describe(imageId), { busy: false });
					await get().refreshHistory();
				} catch (error) {
					set({ busy: false, error: asApiError(error) });
				} finally {
					settle();
				}
			});

		return {
			imageId,
			doc: null,
			load: "loading",
			error: null,
			busy: false,
			selection: [],
			view: INITIAL_VIEW,
			stage: { w: 0, h: 0 },
			history: [],
			changedByOthers: [],
			notice: null,

			async open() {
				try {
					replace(await api.describe(imageId), { error: null });
					await get().refreshHistory();
				} catch (error) {
					set({ load: "failed", error: asApiError(error) });
				}
			},

			send(operations, select) {
				return enqueue(async () => {
					const doc = get().doc;
					if (doc === null) return null;
					const ops =
						typeof operations === "function" ? operations(doc) : operations;
					if (ops.length === 0) return null;
					set({ busy: true, error: null });
					try {
						const result = await api.edit(imageId, doc.revision, ops);
						const fresh = await api.describe(imageId);
						replace(fresh, { busy: false });
						const picked = select?.(result);
						if (picked !== undefined) set({ selection: picked });
						await get().refreshHistory();
						return result;
					} catch (error) {
						const failure = asApiError(error);
						set({ busy: false, error: failure });
						// A refused batch leaves the screen as it was; a stale base means another actor moved on.
						if (failure.code === "E_REVISION_CONFLICT") {
							try {
								replace(await api.describe(imageId));
							} catch {
								// The next event or poll catches up.
							}
						}
						return null;
					} finally {
						settle();
					}
				});
			},

			importFile(source) {
				return enqueue(async () => {
					const doc = get().doc;
					if (doc === null) return null;
					set({ busy: true, error: null });
					try {
						const target = {
							image_id: imageId,
							base_revision: doc.revision,
							...(source.name === undefined ? {} : { name: source.name }),
						};
						const result =
							"path" in source
								? await api.importImage({ ...target, path: source.path })
								: await api.importFile(source.file, target);
						replace(await api.describe(imageId), { busy: false });
						if (result.layer_id !== undefined)
							set({ selection: [result.layer_id] });
						await get().refreshHistory();
						return result;
					} catch (error) {
						const failure = asApiError(error);
						set({ busy: false, error: failure });
						if (failure.code === "E_REVISION_CONFLICT") {
							try {
								replace(await api.describe(imageId));
							} catch {
								// The next event or poll catches up.
							}
						}
						return null;
					} finally {
						settle();
					}
				});
			},

			async undo() {
				await step("undo");
			},
			async redo() {
				await step("redo");
			},

			async poll() {
				await catchUp();
			},

			async receive(event) {
				if (event.name !== "image.updated") return;
				seenRevision = Math.max(seenRevision, event.revision);
				const doc = get().doc;
				if (doc === null || event.revision <= doc.revision) return;
				// While a command of this window runs, its own answer may be this very revision: settle() looks again.
				if (get().busy) return;
				await catchUp();
			},

			async refreshHistory() {
				try {
					const result = await api.history(imageId);
					set({ history: result.entries });
				} catch {
					// The list is a convenience; the document is what matters.
				}
			},

			select(ids) {
				set({ selection: ids });
			},
			setView(view) {
				set({ view: { ...get().view, ...view } });
			},
			setStage(stage) {
				const now = get().stage;
				if (now.w !== stage.w || now.h !== stage.h) set({ stage });
			},
			zoomBy(factor, at) {
				const { doc, view, stage } = get();
				if (doc === null) return;
				const size = { w: doc.width, h: doc.height };
				const point = at ?? { x: stage.w / 2, y: stage.h / 2 };
				set({ view: zoomAbout(view, stage, size, factor, point) });
			},
			fitView() {
				set({ view: INITIAL_VIEW });
			},
			actualSize() {
				const { doc, view, stage } = get();
				if (doc === null) return;
				const size = { w: doc.width, h: doc.height };
				const now = effectiveZoom(view, stage, size);
				set({
					view: {
						...zoomAbout(view, stage, size, 1 / now, {
							x: stage.w / 2,
							y: stage.h / 2,
						}),
						panX: 0,
						panY: 0,
					},
				});
			},
			clearError() {
				set({ error: null });
			},
			clearNotice() {
				set({ notice: null, changedByOthers: [] });
			},
		};
	});
}
