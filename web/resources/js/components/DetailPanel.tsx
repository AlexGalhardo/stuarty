import { type ReactNode, useId } from "react";

import { cn } from "@/lib/cn";

// The detail side of a master-detail screen: the list of ListRow on the left and what the chosen row holds on the
// right. It is a complementary region named by its title; with nothing chosen the caller passes `empty` instead of
// children. It holds no behaviour and no copy of its own.
export function DetailPanel({
	title,
	actions,
	empty,
	children,
	className,
}: {
	title?: ReactNode;
	actions?: ReactNode;
	// What to say when no row is chosen; when given, the children are not drawn.
	empty?: string;
	children?: ReactNode;
	className?: string;
}): ReactNode {
	const titleId = useId();
	return (
		<aside
			data-slot="detail-panel"
			aria-labelledby={title === undefined ? undefined : titleId}
			className={cn(
				"flex min-w-0 flex-col gap-4 rounded-lg border border-border bg-surface p-5",
				className,
			)}
		>
			{title === undefined && actions === undefined ? null : (
				<div className="flex flex-wrap items-start justify-between gap-3">
					<h2 id={titleId} className="min-w-0 text-lg font-semibold text-text">
						{title}
					</h2>
					{actions ? (
						<div className="flex flex-wrap items-center gap-2">{actions}</div>
					) : null}
				</div>
			)}
			{empty !== undefined ? (
				<p className="text-sm text-text-muted">{empty}</p>
			) : (
				children
			)}
		</aside>
	);
}
