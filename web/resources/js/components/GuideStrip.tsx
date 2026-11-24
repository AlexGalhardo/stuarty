import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

// A guide inside the screen it explains: a title and the steps as an ordered
// list (design-system section 4.2). Children are NumberedStep items. The strip
// is a labelled region, so a screen reader finds it by its title.
export function GuideStrip({
	title,
	children,
	className,
}: {
	title: string;
	children: ReactNode;
	className?: string;
}): ReactNode {
	return (
		<section
			data-slot="guide-strip"
			aria-label={title}
			className={cn(
				"rounded-lg border border-border bg-surface px-4 py-4",
				className,
			)}
		>
			<p className="mb-3 text-sm font-semibold text-text-muted">{title}</p>
			<ol className="grid gap-4 sm:grid-cols-3">{children}</ol>
		</section>
	);
}
