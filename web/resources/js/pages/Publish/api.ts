import { fromResponse, networkError } from "../../editor/api/errors";
import { HttpTransport } from "../../editor/api/http";
import { TransportError } from "../../editor/api/transport";
import type {
	ListFilter,
	Publication,
	PublicationCreateInput,
	PublicationList,
	PublicationUpdateInput,
	ScheduleInput,
} from "./types";

const READ_MS = 15_000;
const WRITE_MS = 30_000;

export const STREAM = "/publications/events";

export interface PublishApi {
	list(filter: ListFilter): Promise<PublicationList>;
	create(input: PublicationCreateInput): Promise<Publication>;
	update(id: string, input: PublicationUpdateInput): Promise<Publication>;
	schedule(id: string, input: ScheduleInput): Promise<Publication>;
	sendNow(id: string, targetId: string): Promise<Publication>;
	retry(id: string, targetId: string): Promise<Publication>;
	cancel(id: string, targetId?: string): Promise<Publication>;
}

// The calls of the calendar (SDD-020 section 29). They send what the person typed and return what the service
// answers; a refusal is an ApiError with the message and the hint of the core.
export function createPublishApi(
	transport: HttpTransport = new HttpTransport("/api/v1"),
): PublishApi {
	async function call<T>(
		method: "GET" | "POST" | "PUT",
		path: string,
		timeoutMs: number,
		body?: unknown,
	): Promise<T> {
		let response: Awaited<ReturnType<HttpTransport["send"]>>;
		try {
			response = await transport.send({ method, path, body, timeoutMs });
		} catch (error) {
			if (error instanceof TransportError) throw networkError(error.message);
			throw error;
		}
		if (response.status < 200 || response.status > 299) {
			throw fromResponse(response.status, response.body);
		}
		return response.body as T;
	}

	const path = (id: string, suffix = ""): string =>
		`/publications/${encodeURIComponent(id)}${suffix}`;

	return {
		list: (filter) => {
			const search = new URLSearchParams();
			for (const [key, value] of Object.entries(filter)) {
				if (value !== undefined && value !== "") search.set(key, String(value));
			}
			const text = search.toString();
			return call(
				"GET",
				`/publications${text === "" ? "" : `?${text}`}`,
				READ_MS,
			);
		},
		create: (input) => call("POST", "/publications", WRITE_MS, input),
		update: (id, input) => call("PUT", path(id), WRITE_MS, input),
		schedule: (id, input) =>
			call("POST", path(id, "/schedule"), WRITE_MS, input),
		sendNow: (id, targetId) =>
			call("POST", path(id, "/send-now"), WRITE_MS, { target_id: targetId }),
		retry: (id, targetId) =>
			call("POST", path(id, "/retry"), WRITE_MS, { target_id: targetId }),
		cancel: (id, targetId) =>
			call(
				"POST",
				path(id, "/cancel"),
				WRITE_MS,
				targetId === undefined ? {} : { target_id: targetId },
			),
	};
}
