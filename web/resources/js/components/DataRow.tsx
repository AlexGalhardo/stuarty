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
				"flex flex-wrap items-center justify-between gap-x-6 gap-y-1 py-3",
				className,
			)}
		>
			<div className="min-w-0">
				<dt className="text-sm font-medium text-text">{label}</dt>
				{hint ? <p className="text-sm text-text-subtle">{hint}</p> : null}
			</div>
			<dd className="min-w-0 text-sm text-text-muted">{children}</dd>
		</div>
	);
}
