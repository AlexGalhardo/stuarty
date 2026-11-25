import { Check, Circle, CircleDot, Hourglass } from "lucide-react";
import type { ReactNode } from "react";

import { type Status, StatusBadge } from "@/components/StatusBadge";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import { useLocale, useT } from "@/lib/i18n";
import type { StepState, Workflow, WorkflowStep } from "./types";
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

const MARK: Record<StepState, { icon: typeof Check; tone: string }> = {
	done: {
		icon: Check,
		tone: "border-success bg-success-soft text-success-text",
	},
	current: {
		icon: CircleDot,
		tone: "border-accent bg-accent text-text-on-accent",
	},
	waiting: {
		icon: Hourglass,
		tone: "border-warning bg-warning-soft text-warning-text",
	},
	upcoming: {
		icon: Circle,
		tone: "border-border-strong bg-surface text-text-muted",
	},
	skipped: {
		icon: Circle,
		tone: "border-border-strong bg-surface text-text-muted",
	},
	unknown: {
		icon: Circle,
		tone: "border-border-strong bg-surface text-text-muted",
	},
};

function Fact({
	label,
	children,
}: {
	label: string;
	children: string;
}): ReactNode {
	return (
		<div className="min-w-0">
			<dt className="text-xs text-text-subtle">{label}</dt>
			<dd className="break-words text-sm font-medium text-text">{children}</dd>
		</div>
	);
}

// One step as a row, in the order of the final design: the mark of its state with its title and what it is for,
// then who does it, with which tool and at what cost, then its state and, for an optional step, its button. Every
// value is the server's: the row draws them and decides nothing. A step that can be skipped carries the button of
// its state; the server decides whether the call is allowed. The service keeps no skill for a step, so the row has
// no skill column.
function DetailStep({
	step,
	busy,
	onSkip,
}: {
	step: WorkflowStep;
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
	const { icon: Icon, tone } = MARK[step.state] ?? MARK.unknown;
	return (
		<li
			data-slot="trail-detail-step"
			data-step={step.id}
			data-state={step.state}
			aria-current={step.state === "current" ? "step" : undefined}
			className={cn(
				"grid items-center gap-x-6 gap-y-3 rounded-lg border bg-surface p-4 md:grid-cols-[minmax(0,2.2fr)_repeat(3,minmax(0,1fr))_10.5rem]",
				step.state === "current" || step.state === "waiting"
					? "border-accent"
					: "border-border",
			)}
		>
			<div className="flex min-w-0 items-center gap-3">
				<span
					aria-hidden="true"
					className={cn(
						"flex size-8 shrink-0 items-center justify-center rounded-full border",
						tone,
					)}
				>
					<Icon className="size-4" />
				</span>
				<div className="min-w-0">
					<h3 className="flex flex-wrap items-center gap-2 text-base font-semibold text-text">
						{title}
						{step.optional ? (
							<span className="rounded-md bg-surface-hover px-1.5 py-0.5 text-xs font-normal text-text-muted">
								{t("ui.workflow.optional")}
							</span>
						) : null}
					</h3>
					{hint ? <p className="text-sm text-text-muted">{hint}</p> : null}
				</div>
			</div>
			<dl className="contents">
				<Fact label={t("ui.workflow.who")}>{actorWord(t, step)}</Fact>
				<Fact label={t("ui.workflow.tool")}>{toolWord(t, step)}</Fact>
				<Fact label={t("ui.workflow.cost")}>{costWord(t, locale, step)}</Fact>
			</dl>
			<div className="flex flex-wrap items-center gap-2 md:justify-end">
				<StatusBadge status={TONE[step.state] ?? "neutral"}>
					{stateWord(t, step)}
				</StatusBadge>
				{canToggle ? (
					<Button
						type="button"
						variant="ghost"
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
				) : null}
			</div>
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
			{workflow.steps.map((step) => (
				<DetailStep
					key={step.id}
					step={step}
					busy={busyStep === step.id}
					onSkip={onSkip}
				/>
			))}
		</ol>
	);
}
