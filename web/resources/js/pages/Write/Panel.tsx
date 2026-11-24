import { type ReactNode, useId } from "react";

import { cn } from "@/lib/cn";

// The frame every panel of the editor shares: a card with a heading (h2, 18/26) and an optional sentence under it.
// The heading names the region, so a screen reader finds each panel by it. `headingId` stays settable because the
// panels were found by it before this frame existed (the review button moves focus to the revisions heading).
export default function Panel({
	title,
	description,
	testId,
	headingId,
	actions,
	children,
	className,
}: {
	title: string;
	description?: string;
	testId?: string;
	headingId?: string;
	actions?: ReactNode;
	children: ReactNode;
	className?: string;
}): ReactNode {
	const generated = useId();
	const id = headingId ?? generated;
	return (
		<section
			aria-labelledby={id}
			data-testid={testId}
			data-slot="write-panel"
			className={cn(
				"flex min-w-0 flex-col gap-3 rounded-lg border border-border bg-surface p-4",
				className,
			)}
		>
			<div className="flex flex-wrap items-start justify-between gap-2">
				<div className="min-w-0 flex-1">
					<h2
						id={id}
						tabIndex={-1}
						className="text-lg font-semibold text-text outline-none focus-visible:outline-2"
					>
						{title}
					</h2>
					{description ? (
						<p className="mt-1 text-sm text-text-muted">{description}</p>
					) : null}
				</div>
				{actions ? (
					<div className="flex flex-wrap items-center gap-2">{actions}</div>
				) : null}
			</div>
			{children}
		</section>
	);
}
