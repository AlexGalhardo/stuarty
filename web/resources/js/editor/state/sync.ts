import type { ApiClient } from "../api/client";
import { ApiError } from "../api/errors";
import type { EventStream, Timers } from "../api/events";
import type { Change, EditResult, ProjectEvent } from "../api/types";
import {
	type ActivityWriter,
	changedItemIds,
	HIGHLIGHT_MS,
	summarize,
} from "./activityStore";
import { deepEqual } from "./applyDiff";
import type { ReadStore } from "./createStore";
import type { DocumentState, DocumentWriter } from "./documentStore";

// The synchronization state machine of SDD-012 section 5.5 and SDD-006
// section 6, and the only writer of the document store (rule R4). Every
// revision, from an event or from the response of an edit, enters through
// receive().

export const GAP_TIMEOUT_MS = 2000;
export const MAX_RESYNCS_WITHOUT_PROGRESS = 3;

export interface ReceivedRevision {
	revision: number;
	diff?: readonly Change[];
	truncated?: boolean;
	actor?: string;
}

export interface SyncDeps {
	projectId: string;
	client: ApiClient;
	stream: EventStream;
	store: ReadStore<DocumentState>;
	writer: DocumentWriter;
	activity: ActivityWriter;
	// `local:ui` in local mode, `user:<id>` in network mode (SDD-012 section 5.8).
	ownActor: string;
	timers: Timers;
	// The safety net of SDD-012 section 5.4: after every applied diff, fetch
	// the document and compare. Test and development builds only.
	verify?: boolean;
	onMismatch?(revision: number): void;
	// asset.updated and job.updated carry no revision; the stores that use
	// them subscribe here.
	onSideEvent?(event: ProjectEvent): void;
	// Every batch.committed event, before it is applied: the history panel
	// prepends a row for it.
	onBatch?(event: Extract<ProjectEvent, { name: "batch.committed" }>): void;
	// The read model is being reloaded because events were lost.
	onGap?(): void;
	onUnauthorized?(): void;
}

export interface Sync {
	start(): Promise<void>;
	receive(received: ReceivedRevision): void;
	// The response of an edit, undo or redo: the same entry point as the event.
	receiveEdit(result: EditResult): void;
	resync(): Promise<void>;
	// An edit exhausted its retries: show the connection as offline.
	markOffline(): void;
	stop(): void;
	// Resolves when every in-flight load and verification finished (tests).
	settled(): Promise<void>;
}

export function createSync(deps: SyncDeps): Sync {
	const { client, stream, store, writer, activity, timers } = deps;
	const buffer = new Map<number, ReceivedRevision>();
	const inflight = new Set<Promise<unknown>>();
	let gapTimer: unknown;
	let highlightTimer: unknown;
	let generation = 0;
	let resyncsWithoutProgress = 0;
	let stopped = false;
	let loadedOnce = false;
	let started = false;

	function track(promise: Promise<unknown>): void {
		inflight.add(promise);
		void promise.finally(() => inflight.delete(promise));
	}

	function revision(): number | null {
		return store.getState().revision;
	}

	function cancelGapTimer(): void {
		timers.clearTimeout(gapTimer);
		gapTimer = undefined;
	}

	function startGapTimer(): void {
		if (gapTimer !== undefined) return;
		gapTimer = timers.setTimeout(() => {
			gapTimer = undefined;
			void resync();
		}, GAP_TIMEOUT_MS);
	}

	function refreshEndFrame(forRevision: number): void {
		track(
			client
				.getPlanHead(deps.projectId, 0, 1, forRevision)
				.then((head) => {
					if (!stopped && head.revision === revision())
						writer.setEndFrame(head.end_frame);
				})
				.catch(() => {
					// The end stays as it was; the next revision asks again.
				}),
		);
	}

	function verify(forRevision: number): void {
		if (!deps.verify) return;
		track(
			client
				.getProject(deps.projectId)
				.then((fetched) => {
					const state = store.getState();
					if (
						fetched.revision !== forRevision ||
						state.revision !== forRevision
					)
						return;
					if (!deepEqual(state.document, fetched.document)) {
						deps.onMismatch?.(forRevision);
					}
				})
				.catch(() => undefined),
		);
	}

	function noteRemote(received: ReceivedRevision): void {
		if (received.actor === undefined || received.actor === deps.ownActor)
			return;
		activity.record(
			{
				actor: received.actor,
				revision: received.revision,
				at: timers.now(),
				counts: summarize(received.diff),
			},
			changedItemIds(received.diff),
		);
		timers.clearTimeout(highlightTimer);
		highlightTimer = timers.setTimeout(
			() => activity.clearHighlights(),
			HIGHLIGHT_MS,
		);
	}

	// Applies one revision that is exactly current + 1. False means the caller
	// must resync.
	function applyOne(received: ReceivedRevision): boolean {
		if (received.truncated || received.diff === undefined) return false;
		if (!writer.applyRevisionDiff(received.revision, received.diff))
			return false;
		resyncsWithoutProgress = 0;
		noteRemote(received);
		refreshEndFrame(received.revision);
		verify(received.revision);
		return true;
	}

	function receive(received: ReceivedRevision): void {
		const current = revision();
		if (current === null || stopped) return;
		// Already applied: the other copy (event or response) arrived first.
		if (received.revision <= current) return;
		if (received.revision > current + 1) {
			buffer.set(received.revision, received);
			startGapTimer();
			return;
		}
		if (!applyOne(received)) {
			void resync();
			return;
		}
		let next = buffer.get(received.revision + 1);
		while (next !== undefined) {
			buffer.delete(next.revision);
			if (!applyOne(next)) {
				void resync();
				return;
			}
			next = buffer.get(next.revision + 1);
		}
		if (buffer.size === 0) cancelGapTimer();
	}

	// Boot and resync are the same algorithm (SDD-012 section 5.5 steps 1 and 2).
	async function load(): Promise<void> {
		const mine = ++generation;
		stream.close();
		buffer.clear();
		cancelGapTimer();
		writer.setStatus(loadedOnce ? "resyncing" : "loading");
		try {
			const result = await client.getProject(deps.projectId);
			if (mine !== generation || stopped) return;
			if (result.document === undefined) {
				throw new ApiError(500, {
					code: "E_INTERNAL",
					message: "project response has no document",
				});
			}
			writer.replaceDocument(result.revision, result.document, result.state);
			loadedOnce = true;
			writer.setLoadError(null);
			writer.setStatus("ready");
			stream.open(result.revision);
			refreshEndFrame(result.revision);
		} catch (error) {
			if (mine !== generation || stopped) return;
			if (error instanceof ApiError && error.code === "E_UNAUTHORIZED") {
				deps.onUnauthorized?.();
			}
			const network = error instanceof ApiError && error.isNetwork;
			// Without a document there is nothing to show offline: it is a failed
			// load, with the Reload banner (VV-010).
			const noDocument = !loadedOnce;
			if (error instanceof ApiError && (!network || noDocument)) {
				writer.setLoadError({
					code: error.code,
					message: error.message,
					...(error.hint === undefined ? {} : { hint: error.hint }),
				});
			}
			writer.setStatus(network && !noDocument ? "offline" : "failed");
		}
	}

	async function resync(): Promise<void> {
		if (stopped) return;
		if (resyncsWithoutProgress >= MAX_RESYNCS_WITHOUT_PROGRESS) {
			generation++;
			stream.close();
			cancelGapTimer();
			writer.setStatus("failed");
			return;
		}
		resyncsWithoutProgress++;
		deps.onGap?.();
		const promise = load();
		track(promise);
		await promise;
	}

	function onEvent(event: ProjectEvent): void {
		switch (event.name) {
			case "batch.committed":
				deps.onBatch?.(event);
				receive({
					revision: event.revision,
					diff: event.diff,
					truncated: event.diff_truncated === true,
					actor: event.actor,
				});
				break;
			case "resync.required":
				void resync();
				break;
			case "project.updated":
				writer.setState(event.state);
				break;
			default:
				deps.onSideEvent?.(event);
		}
	}

	stream.onEvent(onEvent);
	stream.onState((connection) => {
		writer.setConnection(connection);
		const status = store.getState().status;
		if (connection === "offline" && status === "ready")
			writer.setStatus("offline");
		if (connection === "live" && status === "offline")
			writer.setStatus("ready");
	});
	stream.onRecover(() => void resync());
	stream.onUnauthorized(() => {
		writer.setStatus("failed");
		deps.onUnauthorized?.();
	});

	return {
		async start() {
			if (started) return;
			started = true;
			const promise = load();
			track(promise);
			await promise;
		},
		receive,
		receiveEdit(result) {
			if (!result.applied) return;
			receive({
				revision: result.revision,
				diff: result.diff,
				actor: result.actor,
			});
		},
		resync,
		markOffline: () => writer.setStatus("offline"),
		stop() {
			stopped = true;
			generation++;
			cancelGapTimer();
			timers.clearTimeout(highlightTimer);
			stream.close();
		},
		async settled() {
			while (inflight.size > 0) await Promise.allSettled([...inflight]);
		},
	};
}
