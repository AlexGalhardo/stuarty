import type { ReactNode } from "react";

import type { Failure } from "@/components/Failure";
import { Notice } from "@/components/Notice";
import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import type { ProjectContextInput } from "../../editor/api/types";
import type { Choice } from "./kinds";
import ProjectContextForm from "./ProjectContextForm";
import { emptyContext } from "./projectSchema";

// The second step of a new project: the briefing of what was chosen. It collects the real fields of the project
// (the name, the description of 64 to 1,024 characters, the five parts of the goal and the due date), with the
// words and the examples of the kind, and each error under its field. The project is created when it is sent.
// When the project was made but its first item was refused, the form gives way to a note that leads to the
// project, so that sending it again does not make a second one.
export default function NewProjectBriefing({
	initial,
	kind,
	busy,
	failure,
	created,
	onEdited,
	onSubmit,
	onBack,
}: {
	initial?: ProjectContextInput;
	kind: Choice;
	busy: boolean;
	failure: Failure | null;
	// The id of the project that exists already, when the first item was refused.
	created: string | null;
	onEdited(): void;
	onSubmit(values: ProjectContextInput): void;
	onBack(): void;
}): ReactNode {
	const t = useT();
	return (
		<div className="flex flex-col gap-3" data-kind={kind}>
			<h3 className="text-lg font-semibold text-text">
				{t("ui.projects.new_project.briefing_title")}
			</h3>
			<p className="text-sm text-text-muted">
				{t(`ui.projects.new_project.lead.${kind}`)}
			</p>
			{created === null ? (
				<ProjectContextForm
					kind={kind}
					initial={initial ?? emptyContext()}
					submitLabel={t("ui.projects.create")}
					cancelLabel={t("ui.projects.new_project.back")}
					busy={busy}
					failure={failure}
					autoFocusFirst
					onEdited={onEdited}
					onSubmit={onSubmit}
					onCancel={onBack}
				/>
			) : (
				<Notice
					tone="warning"
					role="alert"
					title={t("ui.projects.new_project.partial")}
					actions={
						<Button asChild variant="outline" size="sm">
							<a href={`/projects/${created}`}>
								{t("ui.projects.new_project.open_created")}
							</a>
						</Button>
					}
				>
					{failure?.message}
				</Notice>
			)}
		</div>
	);
}
