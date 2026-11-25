import { type ReactNode, useState } from "react";

import { type Failure, FailureNote, failureOf } from "@/components/Failure";
import { StatusBadge } from "@/components/StatusBadge";
import { formatDateTime } from "@/lib/format";
import { useLocale, useT } from "@/lib/i18n";
import { createClient } from "../../editor/api/client";
import { HttpTransport } from "../../editor/api/http";
import { TrailDetail } from "../../workflow/TrailDetail";
import type { TextSummary, Workflow, WorkflowStep } from "../../workflow/types";
import { estimatedCostWord, titleWord } from "../../workflow/words";
import FlowFacts from "./FlowFacts";

// What the service says of the path in the summary strip (screens 22 and 33c): the stage it is at, how many steps
// are done, how many wait, the text facts when present, the estimated cost chip, and the next scheduled post as
// plain text on the right. Every value is the service's.
export function WorkflowSummary({
	workflow,
	text,
	textKind,
	next,
}: {
	workflow: Workflow;
	text?: TextSummary | null;
	textKind?: string;
	next?: string | null;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const at = workflow.steps.findIndex((s) => s.id === workflow.current);
	const current = at < 0 ? undefined : workflow.steps[at];
	const done = workflow.steps.filter((s) => s.state === "done").length;
	const waiting = workflow.steps.filter((s) => s.state === "waiting").length;
	const estimated = estimatedCostWord(t, locale, workflow);
	return (
		<div className="flex flex-wrap items-center justify-between gap-x-4 gap-y-2">
			<ul
				data-testid="workflow-summary"
				className="flex flex-wrap items-center gap-2"
			>
				{current === undefined ? (
					<li>
						<StatusBadge status="neutral">
							{t("ui.dashboard.home.progress.no_next")}
						</StatusBadge>
					</li>
				) : (
					<li>
						<StatusBadge status="info" className="px-3 py-1 text-sm">
							{t("ui.workflow.stage", {
								number: at + 1,
								total: workflow.steps.length,
								step: titleWord(t, current),
							})}
						</StatusBadge>
					</li>
				)}
				{done > 0 ? (
					<li>
						<StatusBadge status="success" className="px-3 py-1 text-sm">
							{t("ui.workflow.summary.done", { count: done })}
						</StatusBadge>
					</li>
				) : null}
				{waiting > 0 ? (
					<li>
						<StatusBadge status="waiting" className="px-3 py-1 text-sm">
							{t("ui.workflow.summary.waiting", { count: waiting })}
						</StatusBadge>
					</li>
				) : null}
				{textKind !== undefined ? (
					<li data-testid="flow-text-kind">
						<StatusBadge status="neutral" className="px-3 py-1 text-sm">
							{t(`ui.write.kind.${textKind}`)}
						</StatusBadge>
					</li>
				) : null}
				<FlowFacts text={text} />
				{estimated !== null ? (
					<li data-testid="workflow-cost">
						<StatusBadge status="neutral" className="px-3 py-1 text-sm">
							{estimated}
						</StatusBadge>
					</li>
				) : null}
			</ul>
			{next ? (
				<p
					data-testid="flow-next-post"
					className="text-sm text-text-muted tabular-nums"
				>
					{t("ui.workflow.next_post", {
						date: formatDateTime(next, locale),
					})}
				</p>
			) : null}
		</div>
	);
}

// The path of the project, from its briefing to its publication, as the service drew it: the chips that sum it up
// and every step with who does it, its tool, its state and its cost, and the skip of an optional step. The page
// decides nothing: a skip is a command, and after it the page reads the workflow again; the steps that were
// finished keep their place on the screen.
export default function WorkflowSection({
	projectId,
	workflow,
	text,
	textKind,
	next,
	onChanged,
}: {
	projectId: string;
	workflow: Workflow;
	text?: TextSummary | null;
	textKind?: string;
	next?: string | null;
	onChanged(): void;
}): ReactNode {
	const t = useT();
	const [busyStep, setBusyStep] = useState<string | null>(null);
	const [failure, setFailure] = useState<Failure | null>(null);

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
			<h2 className="sr-only">{t("ui.workflow.label")}</h2>
			<WorkflowSummary
				workflow={workflow}
				text={text}
				textKind={textKind}
				next={next}
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
