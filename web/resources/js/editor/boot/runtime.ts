import { createClient } from "../api/client";
import { createEditQueue, type EditQueue } from "../api/edit";
import { browserTimers, createEventStream, type Timers } from "../api/events";
import type { Transport } from "../api/transport";
import {
	type ActivityState,
	createActivityStore,
} from "../state/activityStore";
import type { ReadStore } from "../state/createStore";
import {
	createDocumentStore,
	type DocumentState,
} from "../state/documentStore";
import { createSync, type Sync } from "../state/sync";
import { createUiStore, type UiState, type UiWriter } from "../state/uiStore";
import { type EditorBootstrap, ownActor } from "./bootstrap";

export interface RuntimeOptions {
	bootstrap: EditorBootstrap;
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
	documents: ReadStore<DocumentState>;
	ui: ReadStore<UiState>;
	uiWriter: UiWriter;
	activity: ReadStore<ActivityState>;
	sync: Sync;
	edits: EditQueue;
	// Called on visibilitychange to visible and on online (SDD-012 5.5 step 7).
	checkStale(): void;
	dispose(): void;
}

export function createRuntime(options: RuntimeOptions): EditorRuntime {
	const { bootstrap, transport } = options;
	const timers = options.timers ?? browserTimers;
	const checked = options.checked ?? false;
	const projectId = bootstrap.project_id;
	const actor = ownActor(bootstrap);

	const client = createClient(transport);
	const { store: documents, writer } = createDocumentStore({
		projectId,
		freeze: checked,
		now: () => timers.now(),
	});
	const { store: activity, writer: activityWriter } = createActivityStore();
	const { store: ui, writer: uiWriter } = createUiStore();

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
		verify: checked,
		onMismatch: options.onMismatch,
		// E_UNAUTHORIZED policy (SDD-012 section 4.4): network mode goes to
		// /login; local mode keeps the failed status and the shell shows the banner.
		onUnauthorized: () => {
			if (bootstrap.mode === "network") {
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
		documents,
		ui,
		uiWriter,
		activity,
		sync,
		edits,
		checkStale: () => stream.checkStale(),
		dispose: () => sync.stop(),
	};
}
