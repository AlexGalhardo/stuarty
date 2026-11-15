import { afterEach, describe, expect, test, vi } from "vitest";

import { createClient } from "./client";
import { HttpTransport } from "./http";
import { TransportError } from "./transport";
import { exportFileUrl, frameUrl, thumbnailUrl } from "./urls";

// The real shapes of the HTTP API (SDD-003 section 6): same origin, the
// session cookie, JSON in and problem+json out.

afterEach(() => vi.unstubAllGlobals());

function stubFetch(status: number, body: unknown): ReturnType<typeof vi.fn> {
	const fn = vi.fn(
		async () =>
			new Response(typeof body === "string" ? body : JSON.stringify(body), {
				status,
			}),
	);
	vi.stubGlobal("fetch", fn);
	return fn;
}

describe("HttpTransport.send", () => {
	test("joins the base, sends the cookie and no Authorization header", async () => {
		const fetchMock = stubFetch(200, { ok: true });
		const transport = new HttpTransport("/api/v1");
		const response = await transport.send({
			method: "GET",
			path: "/projects/prj_x?detail=document",
			timeoutMs: 1000,
		});
		expect(response).toEqual({ status: 200, body: { ok: true } });
		const [url, init] = fetchMock.mock.calls[0] as unknown as [
			string,
			RequestInit,
		];
		expect(url).toBe("/api/v1/projects/prj_x?detail=document");
		expect(init.credentials).toBe("same-origin");
		expect(init.method).toBe("GET");
		const headers = init.headers as Record<string, string>;
		expect(headers.Accept).toBe("application/json");
		expect(headers.Authorization).toBeUndefined();
		expect(init.body).toBeUndefined();
	});

	test("posts a JSON body with its content type", async () => {
		const fetchMock = stubFetch(200, {});
		await new HttpTransport("/api/v1").send({
			method: "POST",
			path: "/projects/prj_x/edit",
			body: { operations: [], base_revision: 3 },
			timeoutMs: 1000,
		});
		const init = fetchMock.mock.calls[0][1] as RequestInit;
		expect((init.headers as Record<string, string>)["Content-Type"]).toBe(
			"application/json",
		);
		expect(JSON.parse(init.body as string)).toEqual({
			operations: [],
			base_revision: 3,
		});
	});

	test("a DELETE with no body still says it is JSON, which a cookie session needs", async () => {
		const fetchMock = stubFetch(200, {});
		await new HttpTransport("/api/v1").send({
			method: "DELETE",
			path: "/keys/key_x",
			timeoutMs: 1000,
		});
		const init = fetchMock.mock.calls[0][1] as RequestInit;
		expect((init.headers as Record<string, string>)["Content-Type"]).toBe(
			"application/json",
		);
		expect(init.body).toBeUndefined();
	});

	test("a problem+json body comes back as the response", async () => {
		stubFetch(409, {
			type: "urn:vukem-video:error:E_OVERLAP",
			code: "E_OVERLAP",
			message: "overlap",
			retryable: false,
			request_id: "01JC",
		});
		const client = createClient(new HttpTransport("/api/v1"));
		await expect(client.getProject("prj_x")).rejects.toMatchObject({
			code: "E_OVERLAP",
			status: 409,
			requestId: "01JC",
		});
	});

	test("a network failure is a TransportError", async () => {
		vi.stubGlobal(
			"fetch",
			vi.fn(async () => {
				throw new TypeError("failed to fetch");
			}),
		);
		await expect(
			new HttpTransport("/api/v1").send({
				method: "GET",
				path: "/x",
				timeoutMs: 1000,
			}),
		).rejects.toBeInstanceOf(TransportError);
	});

	test("the timeout aborts the request", async () => {
		vi.useFakeTimers();
		vi.stubGlobal(
			"fetch",
			vi.fn(
				(_url: string, init: RequestInit) =>
					new Promise((_resolve, reject) => {
						init.signal?.addEventListener("abort", () =>
							reject(new DOMException("aborted", "AbortError")),
						);
					}),
			),
		);
		const pending = new HttpTransport("/api/v1").send({
			method: "GET",
			path: "/x",
			timeoutMs: 10_000,
		});
		const assertion = expect(pending).rejects.toMatchObject({
			kind: "timeout",
		});
		await vi.advanceTimersByTimeAsync(10_000);
		await assertion;
		vi.useRealTimers();
	});
});

class FakeEventSource {
	static CLOSED = 2;
	static instances: FakeEventSource[] = [];
	readyState = 0;
	onopen: (() => void) | null = null;
	onmessage: ((e: { data: string }) => void) | null = null;
	onerror: (() => void) | null = null;
	listeners = new Map<string, (e: { data: string }) => void>();
	closed = false;
	constructor(public url: string) {
		FakeEventSource.instances.push(this);
	}
	addEventListener(name: string, fn: (e: { data: string }) => void): void {
		this.listeners.set(name, fn);
	}
	close(): void {
		this.closed = true;
	}
}

describe("HttpTransport.openStream", () => {
	test("opens the events URL and maps named events, messages and errors", () => {
		FakeEventSource.instances = [];
		vi.stubGlobal("EventSource", FakeEventSource);
		const seen: [string | null, string][] = [];
		let opened = 0;
		const errors: boolean[] = [];
		const connection = new HttpTransport("/api/v1").openStream(
			"/projects/prj_x/events?since=4",
			{
				onOpen: () => opened++,
				onMessage: (name, data) => seen.push([name, data]),
				onError: (closed) => errors.push(closed),
			},
		);
		const source = FakeEventSource.instances[0];
		expect(source.url).toBe("/api/v1/projects/prj_x/events?since=4");
		source.onopen?.();
		source.onmessage?.({ data: "" });
		source.listeners.get("batch.committed")?.({ data: '{"revision":5}' });
		source.readyState = 0;
		source.onerror?.();
		source.readyState = 2;
		source.onerror?.();
		expect(opened).toBe(1);
		expect(seen).toEqual([
			[null, ""],
			["batch.committed", '{"revision":5}'],
		]);
		expect(errors).toEqual([false, true]);
		connection.close();
		expect(source.closed).toBe(true);
	});
});

describe("endpoints and URLs", () => {
	test("the client calls the routes of web/app/http/endpoints.go", async () => {
		const calls: string[] = [];
		const transport = {
			send: async (r: { method: string; path: string; body?: unknown }) => {
				calls.push(`${r.method} ${r.path}`);
				return { status: 200, body: {} };
			},
			openStream: () => ({ close: () => undefined }),
		};
		const client = createClient(transport);
		await client.getProject("prj_x");
		await client.undo("prj_x");
		await client.redo("prj_x");
		await client.listMedia("prj_x");
		await client.importMedia("prj_x", ["C:/a.mp4"]);
		await client.getLint("prj_x");
		await client.startExport("prj_x");
		await client.startTranscribe("prj_x", "ast_1");
		await client.listJobs("prj_x");
		await client.getJob("job_1");
		expect(calls).toEqual([
			"GET /projects/prj_x?detail=document",
			"POST /projects/prj_x/undo",
			"POST /projects/prj_x/redo",
			"GET /projects/prj_x/media",
			"POST /projects/prj_x/media",
			"GET /projects/prj_x/lint",
			"POST /projects/prj_x/jobs",
			"POST /projects/prj_x/jobs",
			"GET /projects/prj_x/jobs",
			"GET /jobs/job_1",
		]);
	});

	test("binary URLs carry time as a string with a unit", () => {
		expect(frameUrl("prj_x", 375, 540, 12)).toBe(
			"/api/v1/projects/prj_x/frame?at=375f&width=540&rev=12",
		);
		expect(thumbnailUrl("prj_x", "ast_1", 160)).toBe(
			"/api/v1/projects/prj_x/media/ast_1/thumbnail?width=160",
		);
		expect(exportFileUrl("job_1")).toBe("/api/v1/jobs/job_1/file");
	});
});
