import { Bot, Coins, User, Wrench } from "lucide-react";
import type { ReactNode } from "react";

import { type Status, StatusBadge } from "@/components/StatusBadge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import { useLocale, useT } from "@/lib/i18n";
import type { StepActor, StepState, Workflow, WorkflowStep } from "./types";
import {
	actorWord,
	costWord,
	hintWord,
	stateWord,
	titleWord,
	toolWord,
} from "./words";

const TONE: Record<StepState, Status> = {
	done: "success",
	current: "info",
	waiting: "waiting",
	upcoming: "neutral",
	skipped: "neutral",
	unknown: "neutral",
};

const ACTOR_ICON: Record<StepActor, typeof User> = {
	person: User,
	agent: Bot,
	both: Bot,
};

function Fact({
	icon: Icon,
	label,
	children,
}: {
	icon: typeof User;
	label: string;
	children: string;
}): ReactNode {
	return (
		<div className="min-w-0">
			<dt className="flex items-center gap-1.5 text-xs text-text-subtle">
				<Icon aria-hidden="true" className="size-3.5 shrink-0 text-icon" />
				{label}
			</dt>
			<dd className="break-words text-sm text-text">{children}</dd>
		</div>
	);
}

// One step with everything the server says of it: who does it, with which tool, its state and its cost. A step
// that can be skipped carries the button of its state; the server decides whether the call is allowed.
function DetailStep({
	step,
	number,
	busy,
	onSkip,
}: {
	step: WorkflowStep;
	number: number;
	busy: boolean;
	onSkip?: (step: WorkflowStep, restore: boolean) => void;
}): ReactNode {
	const t = useT();
	const locale = useLocale();
	const title = titleWord(t, step);
	const hint = hintWord(t, step);
	const restore = step.state === "skipped";
	const canToggle =
		onSkip !== undefined && step.optional && step.state !== "done";
	return (
		<li
			data-slot="trail-detail-step"
			data-step={step.id}
			data-state={step.state}
			aria-current={step.state === "current" ? "step" : undefined}
			className={cn(
				"flex flex-col gap-3 rounded-lg border bg-surface p-4",
				step.state === "current" || step.state === "waiting"
					? "border-accent"
					: "border-border",
			)}
		>
			<div className="flex flex-wrap items-start justify-between gap-2">
				<div className="min-w-0">
					<h3 className="text-lg font-semibold text-text">
						<span className="mr-2 tabular-nums text-text-subtle">
							{number}.
						</span>
						{title}
					</h3>
					{hint ? <p className="text-sm text-text-muted">{hint}</p> : null}
				</div>
				<div className="flex flex-wrap items-center gap-2">
					{step.optional ? (
						<span className="text-xs text-text-subtle">
							{t("ui.workflow.optional")}
						</span>
					) : null}
					<StatusBadge status={TONE[step.state] ?? "neutral"}>
						{stateWord(t, step)}
					</StatusBadge>
				</div>
			</div>
			<dl className="grid gap-3 sm:grid-cols-3">
				<Fact
					icon={ACTOR_ICON[step.actor] ?? User}
					label={t("ui.workflow.who")}
				>
					{actorWord(t, step)}
				</Fact>
				<Fact icon={Wrench} label={t("ui.workflow.tool")}>
					{toolWord(t, step)}
				</Fact>
				<Fact icon={Coins} label={t("ui.workflow.cost")}>
					{costWord(t, locale, step)}
				</Fact>
			</dl>
			{canToggle ? (
				<div>
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={busy}
						data-testid={`${restore ? "restore" : "skip"}-${step.id}`}
						onClick={() => onSkip?.(step, restore)}
					>
						{t(restore ? "ui.workflow.restore" : "ui.workflow.skip")}
						<span className="sr-only">
							{" "}
							{t(
								restore ? "ui.workflow.restore_step" : "ui.workflow.skip_step",
								{ step: title },
							)}
						</span>
					</Button>
				</div>
			) : null}
		</li>
	);
}

// The workflow of a project with the detail of every step. The steps keep their keys, so a step that was finished
// is not drawn again when a medium adds new ones around it.
export function TrailDetail({
	workflow,
	busyStep,
	onSkip,
	className,
}: {
	workflow: Workflow;
	busyStep?: string | null;
	onSkip?: (step: WorkflowStep, restore: boolean) => void;
	className?: string;
}): ReactNode {
	const t = useT();
	return (
		<ol
			data-slot="trail-detail"
			data-kind={workflow.kind}
			aria-label={t("ui.workflow.label")}
			className={cn("flex flex-col gap-3", className)}
		>
			{workflow.steps.map((step, index) => (
				<DetailStep
					key={step.id}
					step={step}
					number={index + 1}
					busy={busyStep === step.id}
					onSkip={onSkip}
				/>
			))}
		</ol>
	);
}
