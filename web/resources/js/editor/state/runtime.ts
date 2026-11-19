import { type ApiClient, createClient } from "../api/client";
import { createEditQueue, type EditQueue } from "../api/edit";
import { ApiError } from "../api/errors";
import { browserTimers, createEventStream, type Timers } from "../api/events";
import { createSharedEventStream } from "../api/eventsLock";
import type { Transport } from "../api/transport";
import { guardUnauthorized, scopedToTimeline } from "../api/transport";
import type { Job, ProjectEvent } from "../api/types";
import {
	type ActivityState,
	createActivityStore,
} from "../state/activityStore";
import type { ReadStore } from "./createStore";
import { createDocumentStore, type DocumentState } from "./documentStore";
import {
	createEffectiveStore,
	type EffectiveState,
	type EffectiveWriter,
} from "./effectiveStore";
import {
	createGhostStore,
	type GhostState,
	type GhostWriter,
} from "./ghostStore";
import {
	createHistory,
	type HistoryController,
	type HistoryState,
} from "./historyStore";
import { createJobsStore, type JobsState, type JobsWriter } from "./jobsStore";
import {
	createLibraryStore,
	type LibraryState,
	type LibraryWriter,
} from "./libraryStore";
import {
	createNoticeStore,
	type NoticeState,
	type NoticeWriter,
} from "./noticeStore";
import { createSnapStore, type SnapStore } from "./snapStore";
import { createSync, type Sync } from "./sync";
import { createUiStore, type UiState, type UiWriter } from "./uiStore";

export interface RuntimeOptions {
	projectId: string;
	// `local:ui` in local mode, `user:<id>` in network mode (SDD-012 5.8).
	actor: string;
	mode: "local" | "network";
	transport: Transport;
	timers?: Timers;
	// Development and test builds: deep-freeze documents and verify every
	// applied diff against a fetched document (SDD-012 sections 5.3 and 5.4).
	checked?: boolean;
	onMismatch?(revision: number): void;
	// Report vv:edit-apply, the time the store takes to apply an edit answer.
	measure?: boolean;
	// Share one event stream per project among the tabs of the browser (Web
	// Locks and BroadcastChannel). Off in tests and for the fixture server.
	shareStream?: boolean;
}

// Everything the editor shares: the stores, the synchronization state machine
// and the edit queue. Built once per page by boot, handed to components
// through EditorContext.
export interface EditorRuntime {
	projectId: string;
	ownActor: string;
	client: ApiClient;
	documents: ReadStore<DocumentState>;
	ui: ReadStore<UiState>;
	uiWriter: UiWriter;
	// Drag previews and the snap line; written by gestures only.
	ghosts: ReadStore<GhostState>;
	ghostWriter: GhostWriter;
	// The scale and opacity of the plan layers, written by the preview.
	effective: ReadStore<EffectiveState>;
	effectiveWriter: EffectiveWriter;
	// The snap points of the core, cached by revision.
	snap: SnapStore;
	activity: ReadStore<ActivityState>;
	library: ReadStore<LibraryState>;
	libraryWriter: LibraryWriter;
	jobs: ReadStore<JobsState>;
	jobsWriter: JobsWriter;
	// The history of the project: the rows of the panel and whether the
	// current actor has a step to undo or redo (SDD-012 section 15).
	history: ReadStore<HistoryState>;
	historyControls: HistoryController;
	notices: ReadStore<NoticeState>;
	noticeWriter: NoticeWriter;
	// Loads the document and opens the stream, then the media list and the jobs.
	start(): Promise<void>;
	// Imports each path with one request and refreshes the list (SDD-012 13.1).
	importPaths(paths: readonly string[]): Promise<void>;
	// Reloads the media list (after a trash, a restore or an asset.remove).
	reloadLibrary(): Promise<void>;
	// "Move to trash" and "Restore" of a card (SDD-012 section 13.2).
	setAssetTrashed(assetId: string, trashed: boolean): Promise<void>;
	sync: Sync;
	edits: EditQueue;
	// Called on visibilitychange to visible and on online (SDD-012 5.5 step 7).
	checkStale(): void;
	dispose(): void;
}

export function createRuntime(options: RuntimeOptions): EditorRuntime {
	const { transport } = options;
	const timers = options.timers ?? browserTimers;
	const checked = options.checked ?? false;
	const { projectId, actor } = options;

	const toLogin = (): void => {
		location.assign(`/login?next=${encodeURIComponent(location.pathname)}`);
	};
	const client = createClient(
		scopedToTimeline(
			options.mode === "network"
				? guardUnauthorized(transport, toLogin)
				: transport,
			projectId,
		),
	);
	const { store: documents, writer } = createDocumentStore({
		projectId,
		freeze: checked,
		now: () => timers.now(),
	});
	const { store: activity, writer: activityWriter } = createActivityStore();
	const { store: ui, writer: uiWriter } = createUiStore();
	const { store: ghosts, writer: ghostWriter } = createGhostStore();
	const { store: effective, writer: effectiveWriter } = createEffectiveStore();
	const { store: library, writer: libraryWriter } = createLibraryStore();
	const { store: jobs, writer: jobsWriter } = createJobsStore();
	const { store: notices, writer: noticeWriter } = createNoticeStore();

	async function loadLibrary(): Promise<void> {
		try {
			const result = await client.listMedia(projectId);
			libraryWriter.setEntries(result.assets);
		} catch {
			// The list is refetched by the next event; the library keeps what it has.
		}
	}

	async function loadJobs(): Promise<void> {
		try {
			jobsWriter.setAll((await client.listJobs(projectId)).jobs);
		} catch {
			// Jobs reappear with their next event.
		}
	}

	// asset.updated and job.updated carry no revision (SDD-006 section 3).
	function onSideEvent(event: ProjectEvent): void {
		if (event.name === "asset.updated") {
			libraryWriter.patch(event.asset_id, {
				state: event.state,
				progress: event.progress,
			});
			// The event carries no error: a failed card needs the entry again for its
			// code, message and hint (SDD-012 section 13).
			if (event.state === "ready" || event.state === "failed")
				void loadLibrary();
		} else if (event.name === "job.updated") {
			const known = jobs.getState().jobs[event.job_id];
			if (known === undefined) {
				void client
					.getJob(event.job_id)
					.then((job) => jobsWriter.upsert(job))
					.catch(() => undefined);
				return;
			}
			jobsWriter.progress(
				event.job_id,
				{
					state: event.state as Job["state"],
					progress: event.progress,
					message: event.message,
				},
				timers.now(),
			);
			if (event.state === "succeeded" || event.state === "failed") {
				void client
					.getJob(event.job_id)
					.then((job) => jobsWriter.upsert(job))
					.catch(() => undefined);
			}
		}
	}

	const ownStream = createEventStream({
		transport,
		projectId,
		getRevision: () => documents.getState().revision ?? 0,
		probe: async () => {
			try {
				const response = await transport.send({
					method: "GET",
					path: `/projects/${encodeURIComponent(projectId)}`,
					timeoutMs: 10_000,
				});
				return response.status;
			} catch {
				return 0;
			}
		},
		timers,
	});

	const stream =
		options.shareStream && typeof navigator.locks !== "undefined"
			? createSharedEventStream({
					projectId,
					stream: ownStream,
					getRevision: () => documents.getState().revision ?? 0,
					locks: navigator.locks,
					openChannel: (name) => new BroadcastChannel(name),
				})
			: ownStream;

	const historyControls = createHistory({ client, projectId, timers });

	const sync = createSync({
		projectId,
		client,
		stream,
		store: documents,
		writer,
		activity: activityWriter,
		ownActor: actor,
		timers,
		onSideEvent,
		onBatch: (event) => historyControls.onBatch(event),
		onGap: () => historyControls.onGap(),
		verify: checked,
		onMismatch: options.onMismatch,
		// E_UNAUTHORIZED policy (SDD-012 section 4.4): network mode goes to
		// /login; local mode keeps the failed status and the shell shows the banner.
		onUnauthorized: () => {
			if (options.mode === "network") {
				toLogin();
			}
		},
	});

	const edits = createEditQueue({
		client,
		projectId,
		newKey: () => crypto.randomUUID(),
		onResult: (result) => {
			const start = performance.now();
			sync.receiveEdit(result);
			if (options.measure) {
				performance.measure("vv:edit-apply", { start, end: performance.now() });
			}
		},
		onNetworkFailure: () => sync.markOffline(),
	});

	// Drop selected ids the read model no longer has (SDD-012 section 7.6).
	documents.subscribe(() => {
		const doc = documents.getState().document;
		if (doc !== null) uiWriter.dropSelection((id) => id in doc.items);
	});

	const snap = createSnapStore({
		client,
		projectId,
		getRevision: () => documents.getState().revision,
	});

	return {
		projectId,
		ownActor: actor,
		client,
		documents,
		ui,
		uiWriter,
		ghosts,
		ghostWriter,
		effective,
		effectiveWriter,
		snap,
		activity,
		library,
		libraryWriter,
		jobs,
		jobsWriter,
		history: historyControls.store,
		historyControls,
		notices,
		noticeWriter,
		async start() {
			await sync.start();
			await Promise.all([loadLibrary(), loadJobs(), historyControls.refresh()]);
		},
		async importPaths(paths) {
			libraryWriter.setImportError(null);
			for (const path of paths) {
				try {
					const result = await client.importMedia(projectId, [path]);
					for (const asset of result.assets) {
						libraryWriter.rememberPath(asset.name, path);
					}
				} catch (error) {
					if (!(error instanceof ApiError)) throw error;
					if (error.isAborted) continue;
					libraryWriter.setImportError({
						code: error.code,
						message: error.message,
						hint: error.hint,
					});
				}
			}
			await loadLibrary();
		},
		reloadLibrary: loadLibrary,
		async setAssetTrashed(assetId, trashed) {
			try {
				await (trashed
					? client.trashMedia(projectId, assetId)
					: client.restoreMedia(projectId, assetId));
			} catch (error) {
				if (!(error instanceof ApiError)) throw error;
				if (error.isAborted) return;
				noticeWriter.push({
					kind: "error",
					code: error.code,
					message: error.message,
					hint: error.hint,
				});
			}
			await loadLibrary();
		},
		sync,
		edits,
		checkStale: () => stream.checkStale(),
		dispose: () => {
			sync.stop();
			historyControls.dispose();
		},
	};
}
