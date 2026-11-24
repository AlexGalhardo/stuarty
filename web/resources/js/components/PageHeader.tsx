import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

// The top of a page: its one h1, a sentence under it and the page actions on
// the right (design-system section 4.2). Every word comes from the caller, so
// the component draws no data and no copy of its own.
export function PageHeader({
	title,
	description,
	actions,
	className,
}: {
	title: string;
	description?: string;
	actions?: ReactNode;
	className?: string;
}): ReactNode {
	return (
		<header
			data-slot="page-header"
			className={cn(
				"flex flex-wrap items-start justify-between gap-x-6 gap-y-3",
				className,
			)}
		>
			<div className="min-w-0 flex-1">
				<h1 className="text-3xl font-semibold text-text">{title}</h1>
				{description ? (
					<p className="mt-1 text-base text-text-muted">{description}</p>
				) : null}
			</div>
			{actions ? (
				<div className="flex flex-wrap items-center gap-2">{actions}</div>
			) : null}
		</header>
	);
}
