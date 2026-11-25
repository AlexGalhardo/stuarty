import { CircleAlert, RefreshCw } from "lucide-react";
import { type ReactNode, useId } from "react";

import { Button } from "@/components/ui/button";
import { cn } from "@/lib/cn";
import { useT } from "@/lib/i18n";
import { NumberedStep } from "./NumberedStep";

// A region that could not be loaded: what failed in a person's words, a "Try again" button and, when the page
// knows the way out, "How to fix" as numbered steps. It never shows a raw server or network message: the caller
// passes its own translated sentence (design-system section 4.2, the error states of the final design).
export function ErrorState({
	title,
	description,
	steps,
	onRetry,
	retryLabel,
	level = 2,
	className,
}: {
	title: string;
	description?: string;
	// How to fix it, in order, in words; drawn under "How to fix".
	steps?: readonly string[];
	// Called by "Try again"; with none the button is not drawn.
	onRetry?: () => void;
	retryLabel?: string;
	level?: 2 | 3;
	className?: string;
}): ReactNode {
	const t = useT();
	const titleId = useId();
	const Heading = level === 2 ? "h2" : "h3";
	return (
		<section
			data-slot="error-state"
			aria-labelledby={titleId}
			role="alert"
			className={cn(
				"flex flex-col gap-4 rounded-lg border border-danger-text/40 bg-danger-soft px-5 py-5",
				className,
			)}
		>
			<div className="flex items-start gap-3">
				<CircleAlert
					aria-hidden="true"
					className="mt-0.5 size-5 shrink-0 text-danger-text"
				/>
				<div className="min-w-0 flex-1">
					<Heading id={titleId} className="text-base font-semibold text-text">
						{title}
					</Heading>
					{description ? (
						<p className="mt-1 text-sm text-text-muted">{description}</p>
					) : null}
				</div>
				{onRetry ? (
					<Button type="button" variant="outline" onClick={onRetry}>
						<RefreshCw aria-hidden="true" />
						{retryLabel ?? t("ui.state.retry")}
					</Button>
				) : null}
			</div>
			{steps !== undefined && steps.length > 0 ? (
				<div>
					<p className="mb-2 text-sm font-semibold text-text">
						{t("ui.state.how_to_fix")}
					</p>
					<ol className="flex flex-col gap-3">
						{steps.map((step, i) => (
							<NumberedStep key={step} number={i + 1} title={step} />
						))}
					</ol>
				</div>
			) : null}
		</section>
	);
}
