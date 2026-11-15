import { type ApiClient, createClient } from "../api/client";
import { createEditQueue, type EditQueue } from "../api/edit";
import { ApiError } from "../api/errors";
import { browserTimers, createEventStream, type Timers } from "../api/events";
import type { Transport } from "../api/transport";
import type { Job, ProjectEvent } from "../api/types";
import {
	type ActivityState,
	createActivityStore,
} from "../state/activityStore";
import type { ReadStore } from "./createStore";
import { createDocumentStore, type DocumentState } from "./documentStore";
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
	activity: ReadStore<ActivityState>;
	library: ReadStore<LibraryState>;
	libraryWriter: LibraryWriter;
	jobs: ReadStore<JobsState>;
	jobsWriter: JobsWriter;
	notices: ReadStore<NoticeState>;
	noticeWriter: NoticeWriter;
	// Loads the document and opens the stream, then the media list and the jobs.
	start(): Promise<void>;
	// Imports each path with one request and refreshes the list (SDD-012 13.1).
	importPaths(paths: readonly string[]): Promise<void>;
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

	const client = createClient(transport);
	const { store: documents, writer } = createDocumentStore({
		projectId,
		freeze: checked,
		now: () => timers.now(),
	});
	const { store: activity, writer: activityWriter } = createActivityStore();
	const { store: ui, writer: uiWriter } = createUiStore();
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
			if (event.state === "ready") void loadLibrary();
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

	const stream = createEventStream({
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
		verify: checked,
		onMismatch: options.onMismatch,
		// E_UNAUTHORIZED policy (SDD-012 section 4.4): network mode goes to
		// /login; local mode keeps the failed status and the shell shows the banner.
		onUnauthorized: () => {
			if (options.mode === "network") {
				location.assign(`/login?next=${encodeURIComponent(location.pathname)}`);
			}
		},
	});

	const edits = createEditQueue({
		client,
		projectId,
		newKey: () => crypto.randomUUID(),
		onResult: (result) => sync.receiveEdit(result),
		onNetworkFailure: () => sync.markOffline(),
	});

	// Drop selected ids the read model no longer has (SDD-012 section 7.6).
	documents.subscribe(() => {
		const doc = documents.getState().document;
		if (doc !== null) uiWriter.dropSelection((id) => id in doc.items);
	});

	return {
		projectId,
		ownActor: actor,
		client,
		documents,
		ui,
		uiWriter,
		activity,
		library,
		libraryWriter,
		jobs,
		jobsWriter,
		notices,
		noticeWriter,
		async start() {
			await sync.start();
			await Promise.all([loadLibrary(), loadJobs()]);
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
					libraryWriter.setImportError({
						code: error.code,
						message: error.message,
						hint: error.hint,
					});
				}
			}
			await loadLibrary();
		},
		sync,
		edits,
		checkStale: () => stream.checkStale(),
		dispose: () => sync.stop(),
	};
}
