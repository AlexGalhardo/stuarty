import { type ReactNode, useCallback, useEffect, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import UntrustedText from "@/components/UntrustedText";
import { Button } from "@/components/ui/button";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import type { ProjectSummary } from "@/types";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";

// The trash of the projects page (SDD-012 section 2.1, RF-005): the projects
// moved to the trash, each with "Restore". The list comes from the API with
// `state=trashed`; the section is empty text when there is none.
export default function Trash({
	version,
	onRestored,
}: {
	// Changes when a project was moved to the trash, to read the list again.
	version: number;
	onRestored(): void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const [projects, setProjects] = useState<ProjectSummary[] | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);

	// biome-ignore lint/correctness/useExhaustiveDependencies: a new version is the reason to read the list again.
	const load = useCallback(async (): Promise<void> => {
		try {
			const result = await createClient(
				new HttpTransport("/api/v1"),
			).listTrashedProjects();
			setProjects(result.projects);
		} catch (error) {
			setFailure(failureOf(error));
		}
	}, [version]);

	useEffect(() => {
		void load();
	}, [load]);

	async function restore(id: string): Promise<void> {
		setFailure(null);
		try {
			await createClient(new HttpTransport("/api/v1")).restoreProject(id);
			await load();
			onRestored();
		} catch (error) {
			setFailure(failureOf(error));
		}
	}

	return (
		<section aria-labelledby="project-trash">
			<h2 id="project-trash" className="text-lg font-semibold">
				{t("ui.projects.trash.title")}
			</h2>
			<FailureNote failure={failure} />
			{projects !== null &&
				(projects.length === 0 ? (
					<p className="mt-4 text-sm text-muted-foreground">
						{t("ui.projects.trash.empty")}
					</p>
				) : (
					<ul className="mt-4 divide-y rounded-xl border bg-card text-card-foreground">
						{projects.map((project) => (
							<li
								key={project.id}
								className="flex items-center justify-between gap-4 p-4"
								data-trashed-id={project.id}
							>
								<div className="min-w-0">
									<p className="truncate font-medium">
										<UntrustedText>{project.name}</UntrustedText>
									</p>
									<p className="text-xs text-muted-foreground">
										{t("ui.projects.updated", {
											date: formatDateTime(project.updated_at, locale),
										})}
									</p>
								</div>
								<Button
									type="button"
									variant="outline"
									onClick={() => void restore(project.id)}
								>
									{t("ui.projects.trash.restore")}
									<span className="sr-only">
										{" "}
										<UntrustedText>{project.name}</UntrustedText>
									</span>
								</Button>
							</li>
						))}
					</ul>
				))}
		</section>
	);
}
