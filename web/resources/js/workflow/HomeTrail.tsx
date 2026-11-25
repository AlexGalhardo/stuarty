import { Check, Circle, CircleDot } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import type { StepState, Workflow, WorkflowStep } from "./types";
import { actorWord, stateWord, titleWord } from "./words";

const MARK: Record<StepState, string> = {
	done: "border-success bg-success-soft text-success-text",
	current: "border-accent bg-accent text-text-on-accent",
	waiting: "border-accent bg-accent text-text-on-accent",
	upcoming: "border-border-strong bg-surface text-text-muted",
	skipped: "border-border-strong bg-surface text-text-muted",
	unknown: "border-border-strong bg-surface text-text-muted",
};

const SEGMENT: Record<StepState, string> = {
	done: "bg-success",
	current: "bg-accent",
	waiting: "bg-accent",
	upcoming: "bg-border-strong",
	skipped: "bg-border-strong",
	unknown: "bg-border-strong",
};

function Mark({ state }: { state: StepState }): ReactNode {
	const Icon =
		state === "done"
			? Check
			: state === "current" || state === "waiting"
				? CircleDot
				: Circle;
	return (
		<span
			aria-hidden="true"
			className={cn(
				"flex size-8 shrink-0 items-center justify-center rounded-full border",
				MARK[state],
			)}
		>
			<Icon className="size-4" />
		</span>
	);
}

// What the line under a step says: its state in words, or who does it when it has not started, and "Optional" first
// for a step the person may skip. Both are the service's words for the step; the line decides nothing.
function note(t: ReturnType<typeof useT>, step: WorkflowStep): string {
	const base =
		step.state === "done" ||
		step.state === "current" ||
		step.state === "waiting"
			? stateWord(t, step)
			: actorWord(t, step);
	return step.optional
		? `${t("ui.workflow.optional")}, ${base.toLowerCase()}`
		: base;
}

// The path of a project as a row: a mark for each step with its line to the next one, the title and a line of
// words under it. From 768 px up it is the row of the final design; on a phone it is the bar of segments and the
// stage the service says is current. The steps are the workflow as it came: none is added, hidden or reordered.
export function HomeTrail({ workflow }: { workflow: Workflow }): ReactNode {
	const t = useT();
	const steps = workflow.steps;
	const at = steps.findIndex((s) => s.id === workflow.current);
	const current = at < 0 ? undefined : steps[at];
	return (
		<>
			<ol
				data-slot="home-trail"
				data-kind={workflow.kind}
				aria-label={t("ui.workflow.label")}
				className="hidden grid-flow-col auto-cols-fr gap-x-2 md:grid"
			>
				{steps.map((step, index) => (
					<li
						key={step.id}
						data-step={step.id}
						data-state={step.state}
						aria-current={step.state === "current" ? "step" : undefined}
						className="flex min-w-0 flex-col gap-2"
					>
						<div className="flex items-center gap-2">
							<Mark state={step.state} />
							{index < steps.length - 1 ? (
								<span
									aria-hidden="true"
									className={cn(
										"h-0.5 flex-1 rounded-full",
										step.state === "done" ? "bg-success" : "bg-border-strong",
									)}
								/>
							) : null}
						</div>
						<div className="min-w-0">
							<p
								className={cn(
									"truncate text-sm text-text",
									step.state === "current" ? "font-semibold" : "font-medium",
								)}
							>
								{titleWord(t, step)}
							</p>
							<p className="truncate text-sm text-text-subtle">
								{note(t, step)}
							</p>
						</div>
					</li>
				))}
			</ol>
			<div
				className="flex flex-col gap-3 md:hidden"
				data-slot="home-trail-compact"
			>
				<div aria-hidden="true" className="flex gap-1.5">
					{steps.map((step) => (
						<span
							key={step.id}
							className={cn("h-1.5 flex-1 rounded-full", SEGMENT[step.state])}
						/>
					))}
				</div>
				{current === undefined ? null : (
					<p className="inline-flex w-fit items-center gap-1.5 rounded-full bg-accent-soft px-3 py-1 text-sm font-medium text-accent-text">
						<CircleDot aria-hidden="true" className="size-4" />
						{t("ui.workflow.stage", {
							number: at + 1,
							total: steps.length,
							step: titleWord(t, current),
						})}
					</p>
				)}
			</div>
		</>
	);
}
