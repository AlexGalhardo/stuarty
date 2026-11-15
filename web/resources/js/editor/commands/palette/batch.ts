import type { EditOutcome, EditQueue } from "../../api/edit";
import type { EditRequest } from "../../api/types";

export type BatchOperation = EditRequest["operations"][number];

export interface BatchEntry {
	// Stable for the life of the entry, so a chip keeps its key after a removal.
	id: number;
	operation: BatchOperation;
}

export function addToBatch(
	batch: readonly BatchEntry[],
	operation: BatchOperation,
): BatchEntry[] {
	const id = batch.reduce((max, entry) => Math.max(max, entry.id), 0) + 1;
	return [...batch, { id, operation }];
}

export function removeFromBatch(
	batch: readonly BatchEntry[],
	id: number,
): BatchEntry[] {
	return batch.filter((entry) => entry.id !== id);
}

// One `edit()` call for the whole batch: one revision and one undo step
// (RF-105). `base_revision` is read here, at the moment of sending, and a run
// gets a new idempotency key from the queue; a preview sends `dry_run`.
export function sendBatch(
	edits: EditQueue,
	revision: number,
	operations: readonly BatchOperation[],
	dryRun: boolean,
): Promise<EditOutcome> {
	return edits.edit([...operations], revision, { dryRun });
}
