import { ApiError } from "./errors";

// SDD-012 section 4.4: the delays of the automatic retries, with up to 20%
// jitter. Only transient failures are retried, always with the same
// idempotency key, so the core replays instead of applying twice.
export const RETRY_DELAYS_MS = [250, 500, 1000] as const;

const TRANSIENT_STATUS = new Set([502, 503, 504]);

export function isTransient(error: ApiError): boolean {
	if (error.code === "E_WORKSPACE_LOCKED") return true;
	if (error.code === "E_NETWORK")
		return error.status === 0 || TRANSIENT_STATUS.has(error.status);
	return false;
}

export function withJitter(delayMs: number, random: () => number): number {
	return Math.round(delayMs * (1 + 0.2 * random()));
}

export interface RetryDeps {
	sleep(ms: number): Promise<void>;
	random(): number;
}

export async function withRetry<T>(
	attempt: () => Promise<T>,
	deps: RetryDeps,
): Promise<T> {
	for (let n = 0; ; n++) {
		try {
			return await attempt();
		} catch (error) {
			if (!(error instanceof ApiError) || !isTransient(error)) throw error;
			const delay = RETRY_DELAYS_MS[n];
			if (delay === undefined) throw error;
			await deps.sleep(withJitter(delay, deps.random));
		}
	}
}
