import { type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { SectionTitle } from "@/components/SectionTitle";
import { useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import { TrailDetail } from "../../workflow/TrailDetail";
import type { Workflow, WorkflowStep } from "../../workflow/types";
import { titleWord } from "../../workflow/words";
import AddMedia from "./AddMedia";

// The path of the project, from its briefing to its publication, as the service drew it: every step with who does
// it, its tool, its state and its cost, the skip of an optional step and the way to add the medium the project
// lacks. The page decides nothing: a skip is a command, and after it, or after a medium is added, the page reads the
// workflow again; the steps that were finished keep their place on the screen.
export default function WorkflowSection({
	projectId,
	projectName,
	workflow,
	onChanged,
}: {
	projectId: string;
	projectName: string;
	workflow: Workflow;
	onChanged(): void;
}): ReactNode {
	const t = useT();
	const [busyStep, setBusyStep] = useState<string | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);
	const next = workflow.steps.find((s) => s.id === workflow.current);

	const skip = async (step: WorkflowStep, restore: boolean): Promise<void> => {
		setBusyStep(step.id);
		setFailure(null);
		try {
			await createClient(new HttpTransport("/api/v1")).skipProjectStep(
				projectId,
				step.id,
				restore,
			);
			onChanged();
		} catch (error) {
			setFailure(failureOf(error));
		} finally {
			setBusyStep(null);
		}
	};

	return (
		<section
			aria-label={t("ui.workflow.label")}
			data-testid="workflow"
			className="flex flex-col gap-3"
		>
			<SectionTitle
				level={2}
				title={t("ui.workflow.label")}
				description={
					next === undefined
						? t("ui.dashboard.home.progress.no_next")
						: t("ui.dashboard.home.progress.next", {
								step: titleWord(t, next),
							})
				}
				actions={
					<AddMedia
						projectId={projectId}
						projectName={projectName}
						onAdded={onChanged}
					/>
				}
			/>
			<FailureNote failure={failure} />
			<TrailDetail
				workflow={workflow}
				busyStep={busyStep}
				onSkip={(step, restore) => void skip(step, restore)}
			/>
		</section>
	);
}
