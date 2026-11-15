import { describe, expect, test } from "vitest";

import { createClient } from "./client";
import { ApiError } from "./errors";
import type {
	Transport,
	TransportRequest,
	TransportResponse,
} from "./transport";
import { peaksUrl, stripUrl } from "./urls";

// A transport that records the requests and answers with a fixed response, so
// each function is checked for its URL, method, timeout and error mapping.
function fake(response: TransportResponse): {
	transport: Transport;
	requests: TransportRequest[];
} {
	const requests: TransportRequest[] = [];
	const transport = {
		send: async (request: TransportRequest) => {
			requests.push(request);
			return response;
		},
	} as unknown as Transport;
	return { transport, requests };
}

const ok = (body: unknown): TransportResponse => ({ status: 200, body });

describe("ALPHA reads and media state", () => {
	test("getCommands and getCommand", async () => {
		const { transport, requests } = fake(ok({ commands: [], text: "" }));
		const client = createClient(transport);
		await client.getCommands();
		await client.getCommand("item.move");
		expect(requests.map((r) => [r.method, r.path, r.timeoutMs])).toEqual([
			["GET", "/commands", 10_000],
			["GET", "/commands/item.move", 10_000],
		]);
	});

	test("getHistory sends the page and the actor, and nothing when there are none", async () => {
		const { transport, requests } = fake(ok({ entries: [] }));
		const client = createClient(transport);
		await client.getHistory("prj_a");
		await client.getHistory("prj_a", {
			actor: "local:cli",
			cursor: "c1",
			limit: 50,
		});
		expect(requests.map((r) => r.path)).toEqual([
			"/projects/prj_a/history",
			"/projects/prj_a/history?actor=local%3Acli&cursor=c1&limit=50",
		]);
		expect(requests.every((r) => r.method === "GET")).toBe(true);
	});

	test("inspectMedia joins the sections", async () => {
		const { transport, requests } = fake(ok({ asset_id: "ast_a" }));
		const client = createClient(transport);
		await client.inspectMedia("prj_a", "ast_a");
		await client.inspectMedia("prj_a", "ast_a", {
			sections: ["probe", "silences"],
			minSilence: "0.5s",
		});
		expect(requests.map((r) => r.path)).toEqual([
			"/projects/prj_a/media/ast_a",
			"/projects/prj_a/media/ast_a?sections=probe%2Csilences&min_silence=0.5s",
		]);
	});

	test("trashMedia and restoreMedia are POSTs with a 30 s timeout", async () => {
		const { transport, requests } = fake(ok({ asset_id: "ast_a", state: "x" }));
		const client = createClient(transport);
		await client.trashMedia("prj_a", "ast_a");
		await client.restoreMedia("prj_a", "ast_a");
		expect(requests.map((r) => [r.method, r.path, r.timeoutMs])).toEqual([
			["POST", "/projects/prj_a/media/ast_a/trash", 30_000],
			["POST", "/projects/prj_a/media/ast_a/restore", 30_000],
		]);
	});

	test("an error envelope becomes an ApiError with its code", async () => {
		const { transport } = fake({
			status: 409,
			body: { code: "E_IN_USE", message: "in use", retryable: false },
		});
		const client = createClient(transport);
		await expect(client.trashMedia("prj_a", "ast_a")).rejects.toBeInstanceOf(
			ApiError,
		);
		await expect(client.trashMedia("prj_a", "ast_a")).rejects.toMatchObject({
			code: "E_IN_USE",
		});
	});

	test("cancelJob is a POST with a 30 s timeout", async () => {
		const { transport, requests } = fake(
			ok({ job_id: "job_a", state: "cancelled" }),
		);
		await createClient(transport).cancelJob("job_a");
		expect(requests.map((r) => [r.method, r.path, r.timeoutMs])).toEqual([
			["POST", "/jobs/job_a/cancel", 30_000],
		]);
	});
});

describe("binary URL builders", () => {
	test("strip and peaks sit on the media prefix", () => {
		expect(stripUrl("prj_a", "ast_a")).toBe(
			"/api/v1/projects/prj_a/media/ast_a/strip",
		);
		expect(peaksUrl("prj_a", "ast_a")).toBe(
			"/api/v1/projects/prj_a/media/ast_a/peaks",
		);
	});
});
