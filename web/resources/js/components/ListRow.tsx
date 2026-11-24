import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

// One row of a list: an optional leading icon or thumbnail, a title, a
// sentence, a meta line and trailing actions. The row holds no behaviour: a
// whole-row link is the caller's, put in the title. The selected row carries
// aria-current and a bar on its left edge, so selection is not colour alone.
export function ListRow({
	title,
	description,
	meta,
	leading,
	trailing,
	selected = false,
	className,
}: {
	title: ReactNode;
	description?: string;
	meta?: string;
	leading?: ReactNode;
	trailing?: ReactNode;
	selected?: boolean;
	className?: string;
}): ReactNode {
	return (
		<li
			data-slot="list-row"
			data-selected={selected ? "true" : undefined}
			aria-current={selected ? "true" : undefined}
			className={cn(
				"flex items-center gap-3 border-l-2 border-transparent px-3 py-3 hover:bg-surface-hover data-[selected=true]:border-accent data-[selected=true]:bg-surface-selected",
				className,
			)}
		>
			{leading ? <div className="shrink-0 text-icon">{leading}</div> : null}
			<div className="min-w-0 flex-1">
				<div className="truncate text-base font-medium text-text">{title}</div>
				{description ? (
					<p className="truncate text-sm text-text-muted">{description}</p>
				) : null}
				{meta ? <p className="text-sm text-text-subtle">{meta}</p> : null}
			</div>
			{trailing ? (
				<div className="flex shrink-0 items-center gap-2">{trailing}</div>
			) : null}
		</li>
	);
}
