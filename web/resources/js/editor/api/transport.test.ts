import { describe, expect, test } from "vitest";

import {
	scopedToTimeline,
	type Transport,
	type TransportRequest,
} from "./transport";

// The editor names its timeline in every call under its address (SDD-003 section 1.7): as a member of a JSON
// object body, so the path of an edit or an undo stays the one every caller and test knows, or in the query.
function recorder(): { transport: Transport; sent: TransportRequest[] } {
	const sent: TransportRequest[] = [];
	return {
		sent,
		transport: {
			send: async (request) => {
				sent.push(request);
				return { status: 200, body: null };
			},
			openStream: () => ({ close: () => {} }),
		},
	};
}

describe("scopedToTimeline", () => {
	test("an object body carries timeline_id and the path is unchanged", async () => {
		const { transport, sent } = recorder();
		const scoped = scopedToTimeline(transport, "prj_t");
		await scoped.send({
			method: "POST",
			path: "/projects/prj_t/edit",
			body: { base_revision: 3, operations: [] },
			timeoutMs: 1000,
		});
		expect(sent[0]?.path).toBe("/projects/prj_t/edit");
		expect(sent[0]?.body).toEqual({
			base_revision: 3,
			operations: [],
			timeline_id: "prj_t",
		});
	});

	test("a request with no body carries it in the query", async () => {
		const { transport, sent } = recorder();
		const scoped = scopedToTimeline(transport, "prj_t");
		await scoped.send({
			method: "GET",
			path: "/projects/prj_t/plan?from=0&until=30",
			timeoutMs: 1000,
		});
		await scoped.send({
			method: "GET",
			path: "/projects/prj_t",
			timeoutMs: 1000,
		});
		expect(sent.map((r) => r.path)).toEqual([
			"/projects/prj_t/plan?from=0&until=30&timeline_id=prj_t",
			"/projects/prj_t?timeline_id=prj_t",
		]);
	});

	test("other paths and a timeline already named are left alone", async () => {
		const { transport, sent } = recorder();
		const scoped = scopedToTimeline(transport, "prj_t");
		const requests: TransportRequest[] = [
			{ method: "GET", path: "/projects/prj_tx/lint", timeoutMs: 1000 },
			{ method: "POST", path: "/jobs/job_1/cancel", body: {}, timeoutMs: 1000 },
			{
				method: "POST",
				path: "/projects/prj_t/undo",
				body: { timeline_id: "prj_other" },
				timeoutMs: 1000,
			},
			{
				method: "GET",
				path: "/projects/prj_t/frame?timeline_id=prj_t",
				timeoutMs: 1000,
			},
		];
		for (const r of requests) await scoped.send(r);
		expect(sent).toEqual(requests);
	});
});
