import type { ApiClient } from "./client";
import { ApiError } from "./errors";
import type { EditRequest, EditResult } from "./types";

export type EditOutcome =
	| { ok: true; result: EditResult }
	| { ok: false; error: ApiError };

export interface EditOptions {
	// Reused across the retries of one logical edit; a new logical edit gets a
	// new key (SDD-012 section 4.3 rule 2).
	idempotencyKey?: string;
}

export interface EditQueueDeps {
	client: ApiClient;
	projectId: string;
	newKey(): string;
	// Called with every applied result, so the synchronization state machine
	// reconciles it with the event of the same revision (SDD-012 section 5.7).
	onResult(result: EditResult): void;
	// Called when the retries of a transient failure ran out.
	onNetworkFailure(): void;
	// Called with the number of queued and in-flight edits whenever it changes.
	onPending?(count: number): void;
}

export interface EditQueue {
	edit(
		operations: EditRequest["operations"],
		baseRevision: number,
		options?: EditOptions,
	): Promise<EditOutcome>;
	undo(): Promise<EditOutcome>;
	redo(): Promise<EditOutcome>;
}

// A FIFO queue with one request in flight per project (SDD-012 section 4.3
// rule 1). The base revision is the one the caller captured when it composed
// the batch, not the one at send time.
export function createEditQueue(deps: EditQueueDeps): EditQueue {
	let tail: Promise<unknown> = Promise.resolve();
	let pending = 0;

	function enqueue(run: () => Promise<EditResult>): Promise<EditOutcome> {
		pending++;
		deps.onPending?.(pending);
		const outcome = tail.then(async (): Promise<EditOutcome> => {
			try {
				const result = await run();
				if (result.applied) deps.onResult(result);
				return { ok: true, result };
			} catch (error) {
				if (!(error instanceof ApiError)) throw error;
				if (error.isNetwork) deps.onNetworkFailure();
				return { ok: false, error };
			} finally {
				pending--;
				deps.onPending?.(pending);
			}
		});
		tail = outcome.catch(() => undefined);
		return outcome;
	}

	return {
		edit(operations, baseRevision, options = {}) {
			const request: EditRequest = {
				operations,
				base_revision: baseRevision,
				idempotency_key: options.idempotencyKey ?? deps.newKey(),
			};
			return enqueue(() => deps.client.edit(deps.projectId, request));
		},
		undo: () => enqueue(() => deps.client.undo(deps.projectId)),
		redo: () => enqueue(() => deps.client.redo(deps.projectId)),
	};
}
