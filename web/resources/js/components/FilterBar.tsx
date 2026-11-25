import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

// The row of filters above a list or a grid: a search field, selects and toggles side by side, with no apply
// button (a filter acts when it changes). It is a labelled search region and wraps on a narrow window; the
// caller puts the controls inside and owns their state.
export function FilterBar({
	label,
	children,
	className,
}: {
	label: string;
	children: ReactNode;
	className?: string;
}): ReactNode {
	return (
		<search
			data-slot="filter-bar"
			aria-label={label}
			className={cn("flex flex-wrap items-center gap-2", className)}
		>
			{children}
		</search>
	);
}
