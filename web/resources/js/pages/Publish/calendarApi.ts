import { fromResponse, networkError } from "../../editor/api/errors";
import { HttpTransport } from "../../editor/api/http";
import { TransportError } from "../../editor/api/transport";
import type {
	CalendarDay,
	CalendarPlan,
	DayInput,
	DuplicateInput,
	SlotInput,
	SlotRule,
} from "./dayTypes";
import type { Publication } from "./types";

const READ_MS = 15_000;
const WRITE_MS = 30_000;

export interface CalendarApi {
	plan(range: PlanRange): Promise<CalendarPlan>;
	setDay(date: string, input: DayInput): Promise<CalendarDay>;
	setQuiet(date: string, zone: string, quiet: boolean): Promise<CalendarDay>;
	saveSlot(input: SlotInput): Promise<SlotRule>;
	removeSlot(id: string): Promise<void>;
	duplicate(id: string, input: DuplicateInput): Promise<Publication>;
}

export interface PlanRange {
	// dates (2026-12-01) or instants; `to` is exclusive
	from: string;
	to: string;
	zone: string;
	publications?: boolean;
}

// The address of the .ics file of a range (SDD-020 section 30A.5). It is a plain link: the browser sends its own
// session, and the service answers the file as an attachment.
export function exportHref(range: Omit<PlanRange, "publications">): string {
	const search = new URLSearchParams({
		from: range.from,
		to: range.to,
		zone: range.zone,
	});
	return `/api/v1/calendar/export?${search.toString()}`;
}

// The calls of the day of the calendar (SDD-020 section 30A.7). They send what the person did and return what the
// service answers; a refusal is an ApiError with the message, the path and the hint of the core.
export function createCalendarApi(
	transport: HttpTransport = new HttpTransport("/api/v1"),
): CalendarApi {
	async function call<T>(
		method: "GET" | "PUT" | "POST" | "DELETE",
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

	return {
		plan: (range) => {
			const search = new URLSearchParams({
				from: range.from,
				to: range.to,
				zone: range.zone,
			});
			if (range.publications === true) search.set("publications", "true");
			return call("GET", `/calendar/plan?${search.toString()}`, READ_MS);
		},
		setDay: (date, input) =>
			call(
				"PUT",
				`/calendar/days/${encodeURIComponent(date)}`,
				WRITE_MS,
				input,
			),
		setQuiet: (date, zone, quiet) =>
			call("PUT", `/calendar/quiet/${encodeURIComponent(date)}`, WRITE_MS, {
				zone,
				quiet,
			}),
		saveSlot: (input) => call("PUT", "/calendar/slots", WRITE_MS, input),
		removeSlot: async (id) => {
			await call(
				"DELETE",
				`/calendar/slots/${encodeURIComponent(id)}`,
				WRITE_MS,
			);
		},
		duplicate: (id, input) =>
			call(
				"POST",
				`/publications/${encodeURIComponent(id)}/duplicate`,
				WRITE_MS,
				input,
			),
	};
}
