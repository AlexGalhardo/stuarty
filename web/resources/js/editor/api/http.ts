import {
	type StreamConnection,
	type StreamHandlers,
	type Transport,
	TransportError,
	type TransportRequest,
	type TransportResponse,
} from "./transport";
import {
	APPROVAL_EVENT_NAMES,
	CHAT_EVENT_NAMES,
	EVENT_NAMES,
	PUBLICATION_EVENT_NAMES,
} from "./types";

// The only place that calls fetch and EventSource (rule T7). Same origin, the
// session cookie authenticates, no Authorization header and no token in the
// URL (SDD-012 section 4.3 item 5).
export class HttpTransport implements Transport {
	private readonly base: string;

	constructor(base: string) {
		this.base = base;
	}

	async send(request: TransportRequest): Promise<TransportResponse> {
		const controller = new AbortController();
		let timedOut = false;
		const timer = setTimeout(() => {
			timedOut = true;
			controller.abort();
		}, request.timeoutMs);
		request.signal?.addEventListener("abort", () => controller.abort(), {
			once: true,
		});

		try {
			const response = await fetch(this.base + request.path, {
				method: request.method,
				credentials: "same-origin",
				headers: {
					Accept: "application/json",
					// A call that changes state says it is JSON even with no body (a DELETE):
					// the server refuses a cookie session that does not (SDD-007 section 3.6).
					...(request.body === undefined && request.method === "GET"
						? {}
						: { "Content-Type": "application/json" }),
				},
				body:
					request.body === undefined ? undefined : JSON.stringify(request.body),
				signal: controller.signal,
			});
			let body: unknown = null;
			try {
				body = await response.json();
			} catch {
				// An empty or non-JSON body (a proxy page) is reported as null.
			}
			return { status: response.status, body };
		} catch (error) {
			if (timedOut) throw new TransportError("timeout", "request timed out");
			if (request.signal?.aborted) throw error;
			throw new TransportError("network", "network failure");
		} finally {
			clearTimeout(timer);
		}
	}

	openStream(path: string, handlers: StreamHandlers): StreamConnection {
		const source = new EventSource(this.base + path);
		source.onopen = () => handlers.onOpen();
		source.onmessage = (event: MessageEvent<string>) =>
			handlers.onMessage(null, event.data);
		for (const name of [
			...EVENT_NAMES,
			...APPROVAL_EVENT_NAMES,
			...PUBLICATION_EVENT_NAMES,
			...CHAT_EVENT_NAMES,
		]) {
			source.addEventListener(name, (event) =>
				handlers.onMessage(name, (event as MessageEvent<string>).data),
			);
		}
		source.onerror = () =>
			handlers.onError(source.readyState === EventSource.CLOSED);
		return { close: () => source.close() };
	}
}
