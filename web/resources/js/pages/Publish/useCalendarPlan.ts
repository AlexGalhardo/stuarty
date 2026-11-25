import { useCallback, useEffect, useRef, useState } from "react";

import { type Failure, failureOf } from "@/components/Failure";
import { HttpTransport } from "../../editor/api/http";
import {
	type CalendarApi,
	createCalendarApi,
	type PlanRange,
} from "./calendarApi";
import type { CalendarPlan } from "./dayTypes";

export interface CalendarPlanState {
	// null until the first answer
	plan: CalendarPlan | null;
	failure: Failure | null;
	api: CalendarApi;
	reload(): Promise<void>;
	fail(error: unknown): void;
}

// The days, the slots and, when asked, the publications of a range, as the service answers them. `refresh` is any
// value that changes when a publication did (the page passes the times it last saw), so the slots that a post fills
// are read again.
export function useCalendarPlan(
	range: PlanRange,
	refresh = "",
): CalendarPlanState {
	const [plan, setPlan] = useState<CalendarPlan | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const api = useRef<CalendarApi | null>(null);
	if (api.current === null) {
		api.current = createCalendarApi(new HttpTransport("/api/v1"));
	}
	const client = api.current;
	const wanted = useRef(range);
	wanted.current = range;
	const key = JSON.stringify(range);

	const reload = useCallback(async (): Promise<void> => {
		try {
			setPlan(await client.plan(wanted.current));
			setFailure(null);
		} catch (error) {
			setFailure(failureOf(error));
		}
	}, [client]);

	// biome-ignore lint/correctness/useExhaustiveDependencies: key is the range, as text, and refresh a change of the publications
	useEffect(() => {
		void reload();
	}, [reload, key, refresh]);

	const fail = useCallback(
		(error: unknown) => setFailure(failureOf(error)),
		[],
	);

	return { plan, failure, api: client, reload, fail };
}
