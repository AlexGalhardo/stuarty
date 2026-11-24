import type { LucideIcon } from "lucide-react";
import { type ReactNode, useId } from "react";

import { cn } from "@/lib/cn";

// An empty region says what it is for and what to do next: an icon, a title,
// one sentence and the actions (design-system section 4.2). The region is
// named by its title. Children are the actions or a GuideStrip.
export function EmptyState({
	icon: Icon,
	title,
	description,
	level = 2,
	children,
	className,
}: {
	icon: LucideIcon;
	title: string;
	description: string;
	level?: 2 | 3;
	children?: ReactNode;
	className?: string;
}): ReactNode {
	const titleId = useId();
	const Heading = level === 2 ? "h2" : "h3";
	return (
		<section
			data-slot="empty-state"
			aria-labelledby={titleId}
			className={cn(
				"flex flex-col items-center gap-3 rounded-lg border border-dashed border-border-strong bg-surface px-6 py-12 text-center",
				className,
			)}
		>
			<span className="flex size-12 items-center justify-center rounded-full bg-accent-soft text-accent-text">
				<Icon aria-hidden="true" className="size-6" />
			</span>
			<Heading id={titleId} className="text-lg font-semibold text-text">
				{title}
			</Heading>
			<p className="max-w-prose text-base text-text-muted">{description}</p>
			{children ? (
				<div className="mt-2 flex flex-wrap items-center justify-center gap-2">
					{children}
				</div>
			) : null}
		</section>
	);
}
