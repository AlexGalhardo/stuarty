import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

const LEVELS = {
	2: { Tag: "h2", size: "text-2xl" },
	3: { Tag: "h3", size: "text-lg" },
} as const;

// The title of a block of a page (h2, 24/32) or of a card (h3, 18/26), with an
// optional sentence and actions. The level is the caller's: it keeps the
// outline of the page right.
export function SectionTitle({
	title,
	description,
	actions,
	level = 2,
	className,
}: {
	title: string;
	description?: string;
	actions?: ReactNode;
	level?: 2 | 3;
	className?: string;
}): ReactNode {
	const { Tag, size } = LEVELS[level];
	return (
		<div
			data-slot="section-title"
			className={cn(
				"flex flex-wrap items-end justify-between gap-3",
				className,
			)}
		>
			<div className="min-w-0 flex-1">
				<Tag className={cn(size, "font-semibold text-text")}>{title}</Tag>
				{description ? (
					<p className="mt-1 text-sm text-text-muted">{description}</p>
				) : null}
			</div>
			{actions ? (
				<div className="flex flex-wrap items-center gap-2">{actions}</div>
			) : null}
		</div>
	);
}
