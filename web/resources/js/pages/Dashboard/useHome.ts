import { useEffect, useState } from "react";

import { type Failure, failureOf } from "@/components/Failure";
import type { ProjectSummary } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { Workflow } from "../../workflow/types";

// What the home draws. The service says every part of it: the projects, the workflow of the one in progress and
// how many approvals wait for a person. Nothing here decides which step is next.
export type HomeState =
	| { state: "loading" }
	| { state: "failed"; failure: Failure }
	| {
			state: "ready";
			projects: ProjectSummary[];
			// The first project the service lists, with its workflow; null when the workflow could not be read.
			workflow: Workflow | null;
			// Approvals waiting for a person; null when the queue could not be read.
			approvals: number | null;
	  };

export function useHome(): HomeState {
	const [home, setHome] = useState<HomeState>({ state: "loading" });

	useEffect(() => {
		let live = true;
		const api = createClient(new HttpTransport("/api/v1"));
		(async () => {
			const list = await api.listProjects();
			const first = list.projects[0];
			const [described, queue] = await Promise.allSettled([
				first === undefined
					? Promise.resolve(null)
					: api.describeProject(first.id),
				api.listApprovals("pending"),
			]);
			if (!live) return;
			setHome({
				state: "ready",
				projects: list.projects,
				workflow:
					described.status === "fulfilled"
						? (described.value?.workflow ?? null)
						: null,
				approvals: queue.status === "fulfilled" ? queue.value.total : null,
			});
		})().catch((error: unknown) => {
			if (live) setHome({ state: "failed", failure: failureOf(error) });
		});
		return () => {
			live = false;
		};
	}, []);

	return home;
}
