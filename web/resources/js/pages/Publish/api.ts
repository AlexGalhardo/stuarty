import { apiTransport, callJson, type Transport } from "@/lib/api";
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
	sendNow(
		id: string,
		targetId: string,
		confirmQuietDay?: boolean,
	): Promise<Publication>;
	retry(
		id: string,
		targetId: string,
		confirmQuietDay?: boolean,
	): Promise<Publication>;
	cancel(id: string, targetId?: string): Promise<Publication>;
}

// The calls of the calendar (SDD-020 section 29). They send what the person typed and return what the service
// answers; a refusal is an ApiError with the message and the hint of the core.
export function createPublishApi(
	transport: Transport = apiTransport,
): PublishApi {
	async function call<T>(
		method: "GET" | "POST" | "PUT",
		path: string,
		timeoutMs: number,
		body?: unknown,
	): Promise<T> {
		return callJson<T>(
			transport,
			{ method, path, body, timeoutMs },
			{ retry: false },
		);
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
		sendNow: (id, targetId, confirmQuietDay) =>
			call("POST", path(id, "/send-now"), WRITE_MS, {
				target_id: targetId,
				...(confirmQuietDay === true ? { confirm_quiet_day: true } : {}),
			}),
		retry: (id, targetId, confirmQuietDay) =>
			call("POST", path(id, "/retry"), WRITE_MS, {
				target_id: targetId,
				...(confirmQuietDay === true ? { confirm_quiet_day: true } : {}),
			}),
		cancel: (id, targetId) =>
			call(
				"POST",
				path(id, "/cancel"),
				WRITE_MS,
				targetId === undefined ? {} : { target_id: targetId },
			),
	};
}
