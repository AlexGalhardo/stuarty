import type { ReactNode } from "react";

import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/cn";

// Loading is a static skeleton and one sentence, with no spinner (design-system
// section 2.13). The sentence is announced politely; the skeleton is hidden
// from screen readers.
export function LoadingState({
	message,
	className,
}: {
	message: string;
	className?: string;
}): ReactNode {
	return (
		<div
			data-slot="loading-state"
			role="status"
			className={cn("flex flex-col gap-3", className)}
		>
			<div aria-hidden="true" className="flex flex-col gap-2">
				<Skeleton className="h-4 w-full" />
				<Skeleton className="h-4 w-full" />
				<Skeleton className="h-4 w-2/3" />
			</div>
			<p className="text-sm text-text-muted">{message}</p>
		</div>
	);
}
