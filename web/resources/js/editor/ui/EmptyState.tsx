import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

// An empty region names what to do next (docs/design-system.md section
// 4.3.15): an icon, a title and one sentence. The text comes from the caller.
export default function EmptyState({
	icon: Icon,
	title,
	hint,
}: {
	icon: LucideIcon;
	title: string;
	hint: string;
}): ReactNode {
	return (
		<div
			data-empty-state=""
			className="flex flex-col items-center gap-1 px-4 py-8 text-center"
		>
			<Icon aria-hidden="true" className="size-8 text-muted-foreground" />
			<p className="text-sm font-semibold">{title}</p>
			<p className="text-sm text-muted-foreground">{hint}</p>
		</div>
	);
}
