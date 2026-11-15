import { ERROR_TABLE, type ErrorCode } from "@/generated/errors";

import type { ApiErrorBody, JsonValue } from "./types";

// E_NETWORK exists only in the UI: no envelope arrived (SDD-012 section 4.4).
export type UiErrorCode =
	| ErrorCode
	| "E_NETWORK"
	| "E_UNAUTHORIZED"
	| "E_FORBIDDEN";

export class ApiError extends Error {
	readonly code: UiErrorCode | string;
	readonly status: number;
	readonly hint: string | undefined;
	readonly retryable: boolean;
	readonly opIndex: number | undefined;
	readonly path: string | undefined;
	readonly details: { [key: string]: JsonValue } | undefined;
	readonly requestId: string | undefined;

	constructor(
		status: number,
		body: Pick<ApiErrorBody, "code" | "message"> & Partial<ApiErrorBody>,
	) {
		super(body.message);
		this.name = "ApiError";
		this.status = status;
		this.code = body.code;
		this.hint = body.hint;
		this.retryable = body.retryable ?? false;
		this.opIndex = body.op_index;
		this.path = body.path;
		this.details = body.details;
		this.requestId = body.request_id;
	}

	get isNetwork(): boolean {
		return this.code === "E_NETWORK";
	}

	// The caller closed the request, or its deadline passed (E_CLIENT_CLOSED,
	// HTTP 499): the answer nobody waits for. It is never shown to a person.
	get isAborted(): boolean {
		return this.status === 499 || this.code === "E_CLIENT_CLOSED";
	}
}

export function networkError(message: string): ApiError {
	return new ApiError(0, { code: "E_NETWORK", message, retryable: true });
}

function isRecord(value: unknown): value is Record<string, unknown> {
	return typeof value === "object" && value !== null && !Array.isArray(value);
}

// fromResponse turns a non-2xx response into an ApiError. A body that is not
// the envelope (a proxy 502) becomes E_NETWORK, which the retry policy treats
// as transient; the status keeps the original number.
export function fromResponse(status: number, body: unknown): ApiError {
	if (
		isRecord(body) &&
		typeof body.code === "string" &&
		typeof body.message === "string"
	) {
		const known = body.code in ERROR_TABLE;
		const retryable =
			typeof body.retryable === "boolean"
				? body.retryable
				: known && ERROR_TABLE[body.code as ErrorCode].retryable;
		return new ApiError(status, {
			...(body as unknown as ApiErrorBody),
			retryable,
		});
	}
	return new ApiError(status, {
		code: "E_NETWORK",
		message: `HTTP ${status}`,
		retryable: true,
	});
}
