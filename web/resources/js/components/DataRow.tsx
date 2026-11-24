import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

// A list of name and value pairs: a description list, so a screen reader reads
// each value with its name.
export function DataList({
	children,
	className,
}: {
	children: ReactNode;
	className?: string;
}): ReactNode {
	return (
		<dl
			data-slot="data-list"
			className={cn("divide-y divide-border", className)}
		>
			{children}
		</dl>
	);
}

// One row of a DataList: the name on the left, the value on the right, and an
// optional hint under the name.
export function DataRow({
	label,
	hint,
	children,
	className,
}: {
	label: string;
	hint?: string;
	children: ReactNode;
	className?: string;
}): ReactNode {
	return (
		<div
			data-slot="data-row"
			className={cn(
				"grid grid-cols-[minmax(0,1fr)_minmax(0,auto)] items-center gap-x-6 gap-y-1 py-3",
				className,
			)}
		>
			{/* dt and dd are direct children of the row, which is what a description list allows (axe dlitem); the hint is a second dd under the name. */}
			<dt className="col-start-1 min-w-0 text-sm font-medium text-text">
				{label}
			</dt>
			{hint ? (
				<dd className="col-start-1 min-w-0 text-sm text-text-subtle">{hint}</dd>
			) : null}
			<dd className="col-start-2 row-start-1 row-span-2 min-w-0 self-center text-right text-sm text-text-muted">
				{children}
			</dd>
		</div>
	);
}
