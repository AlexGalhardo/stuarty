import { Lock } from "lucide-react";
import type { ReactNode } from "react";

import { NumberedStep, type StepState } from "@/components/NumberedStep";
import { cn } from "@/lib/cn";

export interface AuthStep {
	label: string;
	state: StepState;
	// The word for a finished step, read by screen readers.
	stateLabel?: string;
}

// The frame of the pages before the studio opens (create the account, keep the recovery key, sign in,
// recover): the one h1, an optional line of steps, the form in a card and a closing line. It holds no
// copy and no behaviour; every word is the caller's.
export function AuthCard({
	title,
	description,
	steps,
	stepsLabel,
	footer,
	children,
	wide = false,
}: {
	title: string;
	description?: string;
	steps?: readonly AuthStep[];
	stepsLabel?: string;
	footer?: string;
	children: ReactNode;
	wide?: boolean;
}): ReactNode {
	return (
		<div
			className={cn(
				"mx-auto flex w-full flex-col gap-6 py-6",
				wide ? "max-w-lg" : "max-w-md",
			)}
		>
			{steps !== undefined && (
				<ol aria-label={stepsLabel} className="grid grid-cols-3 gap-2">
					{steps.map((step, index) => (
						<NumberedStep
							key={step.label}
							number={index + 1}
							title={step.label}
							state={step.state}
							stateLabel={step.stateLabel}
							className="flex-col items-center gap-1 text-center [&_span:last-child]:text-sm"
						/>
					))}
				</ol>
			)}
			<div className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-6 text-text">
				<div>
					<h1 className="text-2xl font-semibold">{title}</h1>
					{description ? (
						<p className="mt-1 text-sm text-text-muted">{description}</p>
					) : null}
				</div>
				{children}
			</div>
			{footer ? (
				<p className="flex items-center justify-center gap-2 text-sm text-text-subtle">
					<Lock aria-hidden="true" className="size-4" />
					{footer}
				</p>
			) : null}
		</div>
	);
}
