import { Check } from "lucide-react";
import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

export type StepState = "todo" | "current" | "done";

const MARK: Record<StepState, string> = {
	todo: "border-border-strong bg-surface text-text-muted",
	current: "border-accent bg-accent text-text-on-accent",
	done: "border-success bg-success-soft text-success-text",
};

// One numbered step of a guide, an item of an ordered list. The state is never
// carried by colour alone: a done step shows a check and its word, the current
// one is announced with aria-current and is set in bold.
export function NumberedStep({
	number,
	title,
	description,
	state = "todo",
	stateLabel,
	className,
}: {
	number: number;
	title: string;
	description?: string;
	state?: StepState;
	// The word for the state ("Done", "Current"), read by screen readers and
	// shown next to the title; the caller translates it.
	stateLabel?: string;
	className?: string;
}): ReactNode {
	return (
		<li
			data-slot="numbered-step"
			data-state={state}
			aria-current={state === "current" ? "step" : undefined}
			className={cn("flex items-start gap-3", className)}
		>
			<span
				aria-hidden="true"
				className={cn(
					"flex size-8 shrink-0 items-center justify-center rounded-full border text-sm font-semibold tabular-nums",
					MARK[state],
				)}
			>
				{state === "done" ? <Check className="size-4" /> : number}
			</span>
			<span className="min-w-0 flex-1">
				<span
					className={cn(
						"block text-base text-text",
						state === "current" ? "font-semibold" : "font-medium",
					)}
				>
					{title}
					{stateLabel ? (
						<span className="ml-2 text-sm font-normal text-text-subtle">
							{stateLabel}
						</span>
					) : null}
				</span>
				{description ? (
					<span className="block text-sm text-text-muted">{description}</span>
				) : null}
			</span>
		</li>
	);
}
