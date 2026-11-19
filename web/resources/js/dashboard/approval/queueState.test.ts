import { describe, expect, test } from "vitest";

import type { ApprovalRequest } from "../../editor/api/types";
import { applyApprovalEvent, parseRequest } from "./queueState";

const request = (
	id: string,
	extra: Partial<ApprovalRequest> = {},
): ApprovalRequest => ({
	id,
	actor: "key:key_1",
	kind: "publish.post",
	summary: "Post the video",
	connection_id: null,
	project_id: null,
	job_id: null,
	payload_ref: "job:job_1",
	estimate: { micros: 0, currency: "" },
	state: "pending",
	decided_by: null,
	rule_id: null,
	created_at: "2026-11-19T12:00:00Z",
	decided_at: null,
	expires_at: "2026-11-26T12:00:00Z",
	...extra,
});

const data = (r: ApprovalRequest): string => JSON.stringify(r);

// SDD-015 section 7: the queue follows the stream without a reload.
describe("the queue follows the approval events", () => {
	test("approval.requested shows a request, newest first", () => {
		const older = request("apr_1", { created_at: "2026-11-19T10:00:00Z" });
		const newer = request("apr_2", { created_at: "2026-11-19T11:00:00Z" });
		const list = applyApprovalEvent([older], "approval.requested", data(newer));
		expect(list.map((r) => r.id)).toEqual(["apr_2", "apr_1"]);
	});

	test("a request the stream sends again (a reconnect) is shown once", () => {
		const r = request("apr_1");
		const list = applyApprovalEvent([r], "approval.requested", data(r));
		expect(list).toHaveLength(1);
	});

	test("approval.decided drops the request, whoever decided", () => {
		const r = request("apr_1");
		const decided = request("apr_1", { state: "approved" });
		expect(applyApprovalEvent([r], "approval.decided", data(decided))).toEqual(
			[],
		);
	});

	test("a request that is not pending is not shown", () => {
		const done = request("apr_1", { state: "rejected" });
		expect(applyApprovalEvent([], "approval.requested", data(done))).toEqual(
			[],
		);
	});

	test("other events and bad data change nothing", () => {
		const r = request("apr_1");
		expect(applyApprovalEvent([r], "job.updated", data(r))).toEqual([r]);
		expect(applyApprovalEvent([r], null, "")).toEqual([r]);
		expect(applyApprovalEvent([r], "approval.requested", "not json")).toEqual([
			r,
		]);
		expect(applyApprovalEvent([r], "approval.requested", "[1]")).toEqual([r]);
	});

	test("parseRequest wants an id and a state", () => {
		expect(parseRequest('{"id":"a","state":"pending"}')?.id).toBe("a");
		expect(parseRequest('{"id":1}')).toBeNull();
		expect(parseRequest("null")).toBeNull();
	});
});
