// The seam between the editor and the network. Everything above it (client,
// edit queue, event stream, sync) is written against this interface, so the
// real endpoints and the fixtures of the tests are interchangeable.

export interface TransportRequest {
	method: "GET" | "POST" | "PUT" | "PATCH" | "DELETE";
	// Relative to the API base, starting with "/": "/projects/prj_x".
	path: string;
	body?: unknown;
	// A file sent as it is, instead of a JSON body (SDD-021 section 10.2). The server reads its bytes whatever
	// the declared type is; the request says application/json because a cookie session needs a type that a page
	// of another origin cannot send without a preflight (SDD-007 section 3.6 step 6).
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

// A transport for the editor of one timeline. The address of the editor is the id of a timeline, and a call
// under /projects/<that id> says it again as timeline_id, so a project with several timelines is never
// ambiguous, whichever of its timelines it is (SDD-003 section 1.7). A request with a JSON object body carries
// it as a member of the body and leaves its path as it was; any other request carries it in the query.
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

// A transport that tells `onUnauthorized` about every 401, so an expired
// session in network mode goes to the login whichever call met it (SDD-012
// section 4.4). The answer is passed on unchanged.
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
