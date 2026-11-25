import { useCallback, useEffect, useState } from "react";

import { type Failure, failureOf } from "@/components/Failure";
import type { ProjectSummary } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { Workflow } from "../../workflow/types";

// How many projects get their workflow read: the one the home continues and the ones it lists beside it.
export const DESCRIBED = 6;

// What the home draws. The service says every part of it: the projects and the workflow of each one the home
// shows. Nothing here decides which step is next; what waits for a person is the inbox the shell already holds.
export type HomeState =
	| { state: "loading" }
	| { state: "failed"; failure: Failure }
	| {
			state: "ready";
			projects: ProjectSummary[];
			// The workflow of each of the first projects the service lists, by project id; null when it could not be read.
			workflows: Record<string, Workflow | null>;
	  };

export function useHome(): HomeState & { retry(): void } {
	const [home, setHome] = useState<HomeState>({ state: "loading" });
	const [attempt, setAttempt] = useState(0);
	const retry = useCallback(() => {
		setHome({ state: "loading" });
		setAttempt((n) => n + 1);
	}, []);

	// biome-ignore lint/correctness/useExhaustiveDependencies: `attempt` only restarts the read (Try again)
	useEffect(() => {
		let live = true;
		const api = createClient(new HttpTransport("/api/v1"));
		(async () => {
			const list = await api.listProjects();
			const shown = list.projects.slice(0, DESCRIBED);
			const described = await Promise.allSettled(
				shown.map((p) => api.describeProject(p.id)),
			);
			if (!live) return;
			const workflows: Record<string, Workflow | null> = {};
			shown.forEach((project, index) => {
				const result = described[index];
				workflows[project.id] =
					result?.status === "fulfilled"
						? (result.value.workflow ?? null)
						: null;
			});
			setHome({ state: "ready", projects: list.projects, workflows });
		})().catch((error: unknown) => {
			if (live) setHome({ state: "failed", failure: failureOf(error) });
		});
		return () => {
			live = false;
		};
	}, [attempt]);

	return { ...home, retry };
}
