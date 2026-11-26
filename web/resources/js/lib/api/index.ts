import { HttpTransport } from "./http";

export { type CallOptions, callJson } from "./call";
export {
	ApiError,
	fromResponse,
	networkError,
	type UiErrorCode,
} from "./errors";
export { HttpTransport } from "./http";
export {
	defaultRetryDeps,
	isTransient,
	RETRY_DELAYS_MS,
	type RetryDeps,
	withJitter,
	withRetry,
} from "./retry";
export {
	guardUnauthorized,
	type StreamConnection,
	type StreamHandlers,
	scopedToTimeline,
	type Transport,
	TransportError,
	type TransportRequest,
	type TransportResponse,
} from "./transport";

export const apiTransport = new HttpTransport("/api/v1");
export const rootTransport = new HttpTransport("");
