import { describe, expect, test } from "vitest";

import {
	errorBody,
	FixtureServer,
	flush,
	ManualTimers,
} from "../fixtures/fixtureServer";
import { createClient } from "./client";
import { createEditQueue } from "./edit";
import { ApiError, fromResponse } from "./errors";
import { type ConnectionState, createEventStream } from "./events";
import { RETRY_DELAYS_MS, withJitter } from "./retry";
import type { EditRequest } from "./types";

function fastDeps() {
	const slept: number[] = [];
	return {
		slept,
		deps: {
			sleep: async (ms: number) => {
				slept.push(ms);
			},
			random: () => 0,
		},
	};
}

describe("errors", () => {
	test("499 and E_CLIENT_CLOSED are an aborted request", () => {
		expect(
			fromResponse(499, {
				code: "E_CLIENT_CLOSED",
				message: "closed",
				retryable: false,
			}).isAborted,
		).toBe(true);
		expect(fromResponse(499, "").isAborted).toBe(true);
		expect(
			fromResponse(409, { code: "E_OVERLAP", message: "x" }).isAborted,
		).toBe(false);
	});

	test("the envelope becomes an ApiError with its members", () => {
		const error = fromResponse(409, {
			code: "E_OVERLAP",
			message: "Item would overlap",
			hint: "Use start 120",
			retryable: false,
			op_index: 0,
			path: "args.start",
			details: { next_free: 120 },
			request_id: "01JC",
		});
		expect(error.code).toBe("E_OVERLAP");
		expect(error.hint).toBe("Use start 120");
		expect(error.opIndex).toBe(0);
		expect(error.requestId).toBe("01JC");
		expect(error.status).toBe(409);
	});

	test("a body that is not the envelope is E_NETWORK", () => {
		const error = fromResponse(502, "<html>bad gateway</html>");
		expect(error.code).toBe("E_NETWORK");
		expect(error.retryable).toBe(true);
	});
});

describe("retry policy", () => {
	test("the delays are 250, 500 and 1000 ms with up to 20 percent jitter", () => {
		expect([...RETRY_DELAYS_MS]).toEqual([250, 500, 1000]);
		expect(withJitter(1000, () => 0)).toBe(1000);
		expect(withJitter(1000, () => 1)).toBe(1200);
	});

	test("a transient failure is retried with the same idempotency key", async () => {
		const server = new FixtureServer();
		const { deps, slept } = fastDeps();
		const bodies: EditRequest[] = [];
		server.editHandler = (request) => {
			bodies.push(request.body as EditRequest);
			return { status: 200, body: { revision: 1, applied: true } };
		};
		server.failures.push(
			{ match: "POST", status: 0, body: null },
			{ match: "POST", status: 503, body: "gateway" },
		);
		const client = createClient(server, deps);
		const request: EditRequest = {
			operations: [{ op: "item.move", args: { start: "1s" } }],
			base_revision: 0,
			idempotency_key: "key-0000001",
		};
		await client.edit(server.projectId, request);
		expect(slept).toEqual([250, 500]);
		expect(bodies.length).toBe(1);
		expect(
			server.requests
				.filter((r) => r.method === "POST")
				.every(
					(r) => (r.body as EditRequest).idempotency_key === "key-0000001",
				),
		).toBe(true);
	});

	test("the retries stop after three and surface E_NETWORK", async () => {
		const server = new FixtureServer();
		const { deps, slept } = fastDeps();
		server.setNetworkDown(true);
		const client = createClient(server, deps);
		let caught: unknown;
		try {
			await client.getProject(server.projectId);
		} catch (error) {
			caught = error;
		}
		expect(caught).toBeInstanceOf(ApiError);
		expect((caught as ApiError).code).toBe("E_NETWORK");
		expect(slept).toEqual([250, 500, 1000]);
	});

	test("an error with a code is not retried", async () => {
		const server = new FixtureServer();
		const { deps, slept } = fastDeps();
		server.failures.push({
			match: "GET",
			status: 409,
			body: { ...errorBody("E_REVISION_CONFLICT", "changed"), retryable: true },
		});
		const client = createClient(server, deps);
		let caught: unknown;
		try {
			await client.getProject(server.projectId);
		} catch (error) {
			caught = error;
		}
		expect((caught as ApiError).code).toBe("E_REVISION_CONFLICT");
		expect(slept).toEqual([]);
	});
});

describe("edit queue", () => {
	test("one edit in flight; base_revision is the captured one", async () => {
		const server = new FixtureServer();
		const order: string[] = [];
		let inFlight = 0;
		let maxInFlight = 0;
		server.editHandler = (request) => {
			inFlight++;
			maxInFlight = Math.max(maxInFlight, inFlight);
			const body = request.body as EditRequest;
			order.push(`${body.operations[0].op}@${body.base_revision}`);
			inFlight--;
			return { status: 200, body: { revision: 1, applied: true, diff: [] } };
		};
		const results: number[] = [];
		const queue = createEditQueue({
			client: createClient(server, fastDeps().deps),
			projectId: server.projectId,
			newKey: () => "key-0000001",
			onResult: (r) => results.push(r.revision),
			onNetworkFailure: () => undefined,
		});
		const a = queue.edit([{ op: "item.split", args: {} }], 3);
		const b = queue.edit([{ op: "item.trim", args: {} }], 3);
		await Promise.all([a, b]);
		expect(order).toEqual(["item.split@3", "item.trim@3"]);
		expect(maxInFlight).toBe(1);
		expect(results.length).toBe(2);
	});

	test("an error outcome carries the ApiError and a network failure is reported", async () => {
		const server = new FixtureServer();
		const { deps } = fastDeps();
		let offline = 0;
		const queue = createEditQueue({
			client: createClient(server, deps),
			projectId: server.projectId,
			newKey: () => "key-0000001",
			onResult: () => undefined,
			onNetworkFailure: () => offline++,
		});
		server.editHandler = () => ({
			status: 409,
			body: errorBody("E_OVERLAP", "overlap"),
		});
		const conflict = await queue.edit([{ op: "item.move", args: {} }], 0);
		expect(conflict.ok).toBe(false);
		if (!conflict.ok) expect(conflict.error.code).toBe("E_OVERLAP");
		expect(offline).toBe(0);

		server.setNetworkDown(true);
		const down = await queue.edit([{ op: "item.move", args: {} }], 0);
		expect(down.ok).toBe(false);
		expect(offline).toBe(1);
	});
});

describe("event stream", () => {
	function make(
		server: FixtureServer,
		timers: ManualTimers,
		revision = { n: 0 },
	) {
		const stream = createEventStream({
			transport: server,
			projectId: server.projectId,
			getRevision: () => revision.n,
			probe: async () => 200,
			timers,
		});
		const states: ConnectionState[] = [];
		stream.onState((s) => states.push(s));
		return { stream, states, revision };
	}

	test("opens with since and goes live; the heartbeat is not an event", async () => {
		const server = new FixtureServer();
		const timers = new ManualTimers();
		const { stream, states } = make(server, timers);
		const names: string[] = [];
		stream.onEvent((e) => names.push(e.name));
		stream.open(7);
		await flush();
		server.heartbeat();
		expect(server.openedWith).toEqual([7]);
		expect(states).toEqual(["live"]);
		expect(names).toEqual([]);
	});

	test("40 seconds of silence reopens with the current revision", async () => {
		const server = new FixtureServer();
		const timers = new ManualTimers();
		const { stream, states, revision } = make(server, timers);
		stream.open(0);
		await flush();
		revision.n = 9;
		await timers.advance(39_999);
		expect(server.openedWith).toEqual([0]);
		await timers.advance(1);
		await flush();
		expect(server.openedWith).toEqual([0, 9]);
		expect(states).toContain("reconnecting");
	});

	test("a heartbeat keeps the connection alive", async () => {
		const server = new FixtureServer();
		const timers = new ManualTimers();
		const { stream } = make(server, timers);
		stream.open(0);
		await flush();
		for (let i = 0; i < 5; i++) {
			await timers.advance(15_000);
			server.heartbeat();
		}
		expect(server.openedWith).toEqual([0]);
	});

	test("checkStale reopens only after the window passed", async () => {
		const server = new FixtureServer();
		const timers = new ManualTimers();
		const { stream } = make(server, timers);
		stream.open(0);
		await flush();
		stream.checkStale();
		expect(server.openedWith).toEqual([0]);
	});

	test("a browser retry is reconnecting; a closed stream is offline and recovers", async () => {
		const server = new FixtureServer();
		const timers = new ManualTimers();
		const { stream, states } = make(server, timers);
		let recovered = 0;
		stream.onRecover(() => recovered++);
		stream.open(0);
		await flush();
		server.failStreams(false);
		expect(states.at(-1)).toBe("reconnecting");
		server.failStreams(true);
		expect(states.at(-1)).toBe("offline");
		await timers.advance(1000);
		expect(recovered).toBe(1);
	});

	test("the reopen delays back off to 15 seconds", async () => {
		const server = new FixtureServer();
		const timers = new ManualTimers();
		const stream = createEventStream({
			transport: server,
			projectId: server.projectId,
			getRevision: () => 0,
			probe: async () => 0,
			timers,
		});
		let probes = 0;
		stream.onRecover(() => probes++);
		stream.open(0);
		await flush();
		server.failStreams(true);
		const waits = [1000, 2000, 4000, 8000, 15_000, 15_000];
		for (const wait of waits) {
			await timers.advance(wait - 1);
			await flush();
			expect(timers.pendingCount).toBe(1);
			await timers.advance(1);
			await flush();
		}
		expect(probes).toBe(0);
	});

	test("a 401 probe reports unauthorized", async () => {
		const server = new FixtureServer();
		const timers = new ManualTimers();
		const stream = createEventStream({
			transport: server,
			projectId: server.projectId,
			getRevision: () => 0,
			probe: async () => 401,
			timers,
		});
		let denied = 0;
		stream.onUnauthorized(() => denied++);
		stream.open(0);
		await flush();
		server.failStreams(true);
		await timers.advance(1000);
		expect(denied).toBe(1);
	});
});
