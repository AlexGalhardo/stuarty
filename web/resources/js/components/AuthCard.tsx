import { Lock, type LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

import { NumberedStep, type StepState } from "@/components/NumberedStep";
import { cn } from "@/lib/cn";

export interface AuthStep {
	label: string;
	state: StepState;
	// The word for a finished step, read by screen readers.
	stateLabel?: string;
}

// The mark of the product, centred above the card, in the light or the dark form of the theme.
export function Wordmark({ className }: { className?: string }): ReactNode {
	return (
		<span className={cn("flex items-center justify-center", className)}>
			<img
				src="/logo-wordmark-light.png"
				loading="lazy"
				alt=""
				className="h-8 w-auto dark:hidden"
			/>
			<img
				src="/logo-wordmark-dark.png"
				loading="lazy"
				alt=""
				className="hidden h-8 w-auto dark:block"
			/>
		</span>
	);
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
	icon: Icon,
	iconTone = "accent",
	children,
	wide = false,
}: {
	title: string;
	description?: string;
	steps?: readonly AuthStep[];
	stepsLabel?: string;
	footer?: string;
	// A tile beside the title (the lock of a locked studio); the tone follows the final design.
	icon?: LucideIcon;
	iconTone?: "accent" | "warning";
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
			<Wordmark />
			{steps !== undefined && (
				<ol
					aria-label={stepsLabel}
					className="flex flex-wrap items-center justify-center gap-x-8 gap-y-2"
				>
					{steps.map((step, index) => (
						<NumberedStep
							key={step.label}
							number={index + 1}
							title={step.label}
							state={step.state}
							stateLabel={step.stateLabel}
							className="flex-none items-center gap-2 [&>span:last-child>span:first-child]:text-sm [&>span:first-child]:size-6 [&>span:first-child]:text-xs"
						/>
					))}
				</ol>
			)}
			<div className="flex flex-col gap-4 rounded-lg border border-border bg-surface p-6 text-text">
				<div className="flex items-center gap-3">
					{Icon === undefined ? null : (
						<span
							aria-hidden="true"
							className={cn(
								"flex size-12 shrink-0 items-center justify-center rounded-lg",
								iconTone === "warning"
									? "bg-warning-soft text-warning-text"
									: "bg-accent-soft text-accent-text",
							)}
						>
							<Icon className="size-6" />
						</span>
					)}
					<div className="min-w-0">
						<h1 className="text-2xl font-semibold">{title}</h1>
						{description ? (
							<p className="mt-1 text-sm text-text-muted">{description}</p>
						) : null}
					</div>
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
