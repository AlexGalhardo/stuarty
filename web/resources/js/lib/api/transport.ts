// Shared network transport seam for all studio domains.

export interface TransportRequest {
	method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
	// Relative to the API base, starting with "/": "/projects/prj_x".
	path: string;
	body?: unknown;
	// A file sent as it is, instead of a JSON body (SDD-021 section 10.2).
	file?: Blob;
	timeoutMs: number;
	signal?: AbortSignal;
}

export interface TransportResponse {
	status: number;
	// The decoded JSON body, or null when there is none or it is not JSON.
	body: unknown;
}

// A failure with no HTTP response: the network is down or the timeout fired.
export class TransportError extends Error {
	readonly kind: "network" | "timeout";

	constructor(kind: "network" | "timeout", message: string) {
		super(message);
		this.name = "TransportError";
		this.kind = kind;
	}
}

export interface StreamHandlers {
	// The connection is (re)established.
	onOpen(): void;
	// `name` is null for the unnamed message (preamble and heartbeat).
	onMessage(name: string | null, data: string): void;
	// `closed` is true when the browser gave up (readyState CLOSED); otherwise it
	// is retrying by itself.
	onError(closed: boolean): void;
}

export interface StreamConnection {
	close(): void;
}

export interface Transport {
	send(request: TransportRequest): Promise<TransportResponse>;
	openStream(path: string, handlers: StreamHandlers): StreamConnection;
}

export function scopedToTimeline(
	transport: Transport,
	timelineId: string,
): Transport {
	const prefix = `/projects/${encodeURIComponent(timelineId)}`;
	const scoped = (request: TransportRequest): TransportRequest => {
		const { path, body } = request;
		if (
			path !== prefix &&
			!path.startsWith(`${prefix}/`) &&
			!path.startsWith(`${prefix}?`)
		)
			return request;
		if (/[?&]timeline_id=/.test(path)) return request;
		if (
			body !== null &&
			typeof body === "object" &&
			Object.getPrototypeOf(body) === Object.prototype
		) {
			if ("timeline_id" in body) return request;
			return { ...request, body: { ...body, timeline_id: timelineId } };
		}
		return {
			...request,
			path: `${path}${path.includes("?") ? "&" : "?"}timeline_id=${encodeURIComponent(timelineId)}`,
		};
	};
	return {
		send: (request) => transport.send(scoped(request)),
		openStream: (path, handlers) => transport.openStream(path, handlers),
	};
}

export function guardUnauthorized(
	transport: Transport,
	onUnauthorized: () => void,
): Transport {
	return {
		async send(request) {
			const response = await transport.send(request);
			if (response.status === 401) onUnauthorized();
			return response;
		},
		openStream: (path, handlers) => transport.openStream(path, handlers),
	};
}
