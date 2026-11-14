import type { ProjectDocument } from "@/generated/timeline";

import type { ConnectionState } from "../api/events";
import type { Change, ProjectState } from "../api/types";
import { applyDiff, type DeepReadonly, deepEqual } from "./applyDiff";
import { createStore, type ReadStore } from "./createStore";

export type SyncStatus =
	| "loading"
	| "ready"
	| "resyncing"
	| "offline"
	| "failed";

// The read model (rule T2): the document at a revision, and the state of the
// synchronization that keeps it. Nothing else of the project lives in the
// browser.
export interface DocumentState {
	projectId: string;
	status: SyncStatus;
	connection: ConnectionState;
	revision: number | null;
	document: DeepReadonly<ProjectDocument> | null;
	state: ProjectState;
	// The end_frame of the last plan window the core answered (SDD-004 section
	// 10): the browser derives no end from the items.
	endFrame: number;
	lastEventAt: number;
}

// The write side. Only editor/state/sync.ts imports it (rule R4).
export interface DocumentWriter {
	replaceDocument(
		revision: number,
		document: ProjectDocument,
		state?: ProjectState,
	): void;
	// Requires revision === current + 1; false when the diff is unpatchable.
	applyRevisionDiff(revision: number, diff: readonly Change[]): boolean;
	setStatus(status: SyncStatus): void;
	setConnection(connection: ConnectionState): void;
	setState(state: ProjectState): void;
	setEndFrame(endFrame: number): void;
}

export interface DocumentStoreOptions {
	projectId: string;
	// Deep-freeze every document in development and test builds, so a stray
	// write throws instead of silently diverging.
	freeze: boolean;
	now(): number;
}

function deepFreeze<T>(value: T): T {
	if (typeof value === "object" && value !== null && !Object.isFrozen(value)) {
		Object.freeze(value);
		for (const child of Object.values(value)) deepFreeze(child);
	}
	return value;
}

type Dict = Record<string, unknown>;

// Reuses the previous object for every entry whose JSON is equal, so React
// components keep their props identity (SDD-012 section 5.3 item 1).
function share<T>(previous: unknown, next: T): T {
	if (previous === undefined || previous === null) return next;
	if (deepEqual(previous, next)) return previous as T;
	if (
		typeof next === "object" &&
		next !== null &&
		!Array.isArray(next) &&
		typeof previous === "object" &&
		!Array.isArray(previous)
	) {
		const out: Dict = {};
		for (const [key, value] of Object.entries(next as Dict)) {
			out[key] = share((previous as Dict)[key], value);
		}
		return out as T;
	}
	return next;
}

export function createDocumentStore(options: DocumentStoreOptions): {
	store: ReadStore<DocumentState>;
	writer: DocumentWriter;
} {
	const store = createStore<DocumentState>({
		projectId: options.projectId,
		status: "loading",
		connection: "connecting",
		revision: null,
		document: null,
		state: "active",
		endFrame: 0,
		lastEventAt: options.now(),
	});
	const seal = <T>(value: T): T => (options.freeze ? deepFreeze(value) : value);

	const writer: DocumentWriter = {
		replaceDocument(revision, document, projectState) {
			store.setState((s) => ({
				...s,
				revision,
				document: seal(share(s.document, document)),
				state: projectState ?? s.state,
				lastEventAt: options.now(),
			}));
		},
		applyRevisionDiff(revision, diff) {
			const current = store.getState();
			if (current.document === null || current.revision === null) return false;
			if (revision !== current.revision + 1) return false;
			const patched = applyDiff(current.document, diff);
			if (patched === "unpatchable") return false;
			store.setState((s) => ({
				...s,
				revision,
				document: seal(share(s.document, patched)),
				lastEventAt: options.now(),
			}));
			return true;
		},
		setStatus: (status) => store.setState((s) => ({ ...s, status })),
		setConnection: (connection) =>
			store.setState((s) => ({ ...s, connection })),
		setState: (state) => store.setState((s) => ({ ...s, state })),
		setEndFrame: (endFrame) => store.setState((s) => ({ ...s, endFrame })),
	};

	return {
		store: { getState: store.getState, subscribe: store.subscribe },
		writer,
	};
}
