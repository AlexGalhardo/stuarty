import { Bot, ShieldCheck } from "lucide-react";
import { type ReactNode, useState } from "react";

import { type Failure, failureOf } from "@/components/Failure";
import { NumberedStep } from "@/components/NumberedStep";
import { PageHeader } from "@/components/PageHeader";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import type { ProjectContextInput } from "../../editor/api/types";
import { createPlan } from "./kinds";
import type { NewProjectPick } from "./NewProject";
import NewProjectBriefing from "./NewProjectBriefing";
import { emptyContext } from "./projectSchema";

// The second step of a new project as a page of its own (design 02b, 33b): the steps on the left, the briefing
// form in the middle and what the agent reads on the right. The project is created when the form is sent, with
// the first item the choice calls for (createPlan); a refused field comes back under it. When the project was made
// but its first item was refused, the form gives way to a note that leads to the project.
export default function NewProjectPage({
	pick,
	onBack,
}: {
	pick: NewProjectPick;
	onBack(): void;
}): ReactNode {
	const t = useT();
	const [failure, setFailure] = useState<Failure | null>(null);
	const [busy, setBusy] = useState(false);
	const [created, setCreated] = useState<string | null>(null);

	const submit = async (values: ProjectContextInput): Promise<void> => {
		setFailure(null);
		setBusy(true);
		const plan = createPlan(pick, values.name);
		const api = createClient(new HttpTransport("/api/v1"));
		let id: string;
		try {
			const result = await api.createProject(
				plan.kind === undefined ? values : { ...values, kind: plan.kind },
			);
			id = result.project_id;
		} catch (error) {
			setFailure(failureOf(error));
			setBusy(false);
			return;
		}
		if (plan.item !== undefined) {
			try {
				await api.addProjectItem(id, plan.item);
			} catch (error) {
				setFailure(failureOf(error));
				setCreated(id);
				setBusy(false);
				return;
			}
		}
		location.assign(`/projects/${id}`);
	};

	const summary =
		pick.choice === "text"
			? t(`ui.write.kind.${pick.textType}`)
			: pick.choice === "video"
				? pick.format
				: t(`ui.projects.new_project.choice.${pick.choice}.title`);

	return (
		<div className="mx-auto flex max-w-7xl flex-col gap-6">
			<PageHeader
				title={t(`ui.projects.new_project.page_title.${pick.choice}`)}
				description={t("ui.projects.new_project.page_lead")}
			/>
			<div className="grid gap-6 lg:grid-cols-[14rem_minmax(0,1fr)_18rem]">
				<nav aria-label={t("ui.projects.new_project.steps")}>
					<p className="mb-3 text-sm font-semibold text-accent-text">
						{t("ui.projects.new_project.step_two")}
					</p>
					<ol className="flex flex-col gap-4">
						<NumberedStep
							number={1}
							state="done"
							title={t("ui.projects.new_project.step_kind")}
							description={summary}
							stateLabel={t("ui.workflow.state.done")}
						/>
						<NumberedStep
							number={2}
							state="current"
							title={t("ui.projects.new_project.step_idea")}
							description={t("ui.projects.new_project.step_idea_hint")}
						/>
					</ol>
				</nav>
				<NewProjectBriefing
					initial={{ ...emptyContext(), name: pick.name }}
					kind={pick.choice}
					busy={busy}
					failure={failure}
					created={created}
					onEdited={() => setFailure(null)}
					onSubmit={(values) => void submit(values)}
					onBack={onBack}
				/>
				<aside
					aria-label={t("ui.projects.new_project.reads_title")}
					className="flex h-fit flex-col gap-3 rounded-xl border border-border bg-surface p-5"
				>
					<h2 className="flex items-center gap-2 text-lg font-semibold">
						<Bot aria-hidden="true" className="size-5 text-icon" />
						{t("ui.projects.new_project.reads_title")}
					</h2>
					<p className="text-sm text-text-muted">
						{t("ui.projects.new_project.reads_body")}
					</p>
					<p className="flex items-start gap-2 border-t border-border pt-3 text-sm text-text-muted">
						<ShieldCheck
							aria-hidden="true"
							className="mt-0.5 size-4 shrink-0"
						/>
						{t("ui.projects.new_project.reads_permission")}
					</p>
				</aside>
			</div>
		</div>
	);
}
