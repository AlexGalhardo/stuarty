import { describe, expect, test } from "vitest";
import { callJson } from "./call";
import { ApiError } from "./errors";
import { type Transport, TransportError } from "./transport";

const instantRetry = {
	sleep: async () => undefined,
	random: () => 0,
};

function makeTransport(fn: Transport["send"]): Transport {
	return {
		send: fn,
		openStream: () => ({ close: () => undefined }),
	};
}

describe("callJson", () => {
	test("returns parsed body on 2xx", async () => {
		const t = makeTransport(async () => ({ status: 200, body: { ok: true } }));
		const res = await callJson<{ ok: boolean }>(t, {
			method: "GET",
			path: "/ping",
			timeoutMs: 1000,
		});
		expect(res).toEqual({ ok: true });
	});

	test("retries transient 503 and succeeds", async () => {
		let calls = 0;
		const t = makeTransport(async () => {
			calls++;
			if (calls < 2) return { status: 503, body: null };
			return { status: 200, body: { ok: true } };
		});
		const res = await callJson<{ ok: boolean }>(
			t,
			{ method: "GET", path: "/ping", timeoutMs: 1000 },
			{ retry: instantRetry },
		);
		expect(calls).toBe(2);
		expect(res).toEqual({ ok: true });
	});

	test("does not retry 400 ApiError", async () => {
		let calls = 0;
		const t = makeTransport(async () => {
			calls++;
			return {
				status: 400,
				body: { code: "E_VALIDATION", message: "bad input" },
			};
		});
		await expect(
			callJson(
				t,
				{ method: "POST", path: "/items", timeoutMs: 1000 },
				{ retry: instantRetry },
			),
		).rejects.toBeInstanceOf(ApiError);
		expect(calls).toBe(1);
	});

	test("maps TransportError to E_NETWORK when retry is disabled", async () => {
		const t = makeTransport(async () => {
			throw new TransportError("network", "offline");
		});
		await expect(
			callJson(
				t,
				{ method: "GET", path: "/ping", timeoutMs: 1000 },
				{ retry: false },
			),
		).rejects.toMatchObject({ code: "E_NETWORK", status: 0 });
	});
});
