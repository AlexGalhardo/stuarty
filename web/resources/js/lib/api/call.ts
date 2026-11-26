import { fromResponse, networkError } from "./errors";
import { defaultRetryDeps, type RetryDeps, withRetry } from "./retry";
import {
	type Transport,
	TransportError,
	type TransportRequest,
} from "./transport";

export interface CallOptions {
	// Pass false to disable automatic retry on transient failures, or custom RetryDeps for tests.
	retry?: RetryDeps | false;
}

// Shared JSON request wrapper across all studio API clients: maps TransportError to
// E_NETWORK, non-2xx responses to ApiError via fromResponse, and optionally retries
// transient failures with the unified policy.
export async function callJson<T>(
	transport: Transport,
	request: TransportRequest,
	options?: CallOptions,
): Promise<T> {
	const attempt = async (): Promise<T> => {
		let response: Awaited<ReturnType<Transport["send"]>>;
		try {
			response = await transport.send(request);
		} catch (error) {
			if (error instanceof TransportError) throw networkError(error.message);
			throw error;
		}
		if (response.status < 200 || response.status > 299) {
			throw fromResponse(response.status, response.body);
		}
		return response.body as T;
	};

	if (options?.retry === false) return attempt();
	return withRetry(attempt, options?.retry ?? defaultRetryDeps);
}
