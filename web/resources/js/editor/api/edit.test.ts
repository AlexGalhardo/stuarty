import { describe, expect, test } from "vitest";

import { createEditQueue } from "./edit";
import { ApiError } from "./errors";
import type { EditRequest, EditResult } from "./types";

const result = (revision: number): EditResult =>
	({ applied: true, revision }) as unknown as EditResult;

const conflict = (): ApiError =>
	new ApiError(409, {
		code: "E_REVISION_CONFLICT",
		message: "changed",
		retryable: true,
		details: { base_revision: 10, current_revision: 11 },
	});

function setup(answers: (request: EditRequest, call: number) => EditResult) {
	const requests: EditRequest[] = [];
	let keys = 0;
	const queue = createEditQueue({
		client: {
			edit: async (_id: string, request: EditRequest) => {
				requests.push(request);
				return answers(request, requests.length);
			},
		} as never,
		projectId: "prj_1",
		newKey: () => `key_${++keys}`,
		onResult: () => undefined,
		onNetworkFailure: () => undefined,
	});
	return { queue, requests };
}

const ops = [{ op: "item.move", args: { item_id: "a", start: "5f" } }];

describe("edit queue (SDD-012 section 4.5)", () => {
	test("base_revision is the one the caller captured, never replaced", async () => {
		const { queue, requests } = setup((_r, call) => result(call));
		// A gesture went down at revision 10; the read model is at 12 by release.
		await queue.edit(ops, 10);
		expect(requests[0]?.base_revision).toBe(10);
	});

	test("a conflicted batch is never resent on its own", async () => {
		const { queue, requests } = setup(() => {
			throw conflict();
		});
		const outcome = await queue.edit(ops, 10);
		expect(outcome.ok).toBe(false);
		await new Promise((resolve) => setTimeout(resolve, 20));
		expect(requests).toHaveLength(1);
	});

	test("Apply again is a new logical edit: a new key and the current revision", async () => {
		const { queue, requests } = setup((_r, call) => {
			if (call === 1) throw conflict();
			return result(11);
		});
		await queue.edit(ops, 10);
		await queue.edit(ops, 11);
		expect(requests).toHaveLength(2);
		expect(requests[1]?.base_revision).toBe(11);
		expect(requests[1]?.idempotency_key).not.toBe(requests[0]?.idempotency_key);
		expect(requests[1]?.operations).toEqual(requests[0]?.operations);
	});
});
