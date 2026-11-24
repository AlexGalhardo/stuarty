import type { ReactNode } from "react";

import { cn } from "@/lib/cn";

// The class of a native select on a settings page: the sunken fill and the strong
// boundary of the form controls (design-system section 4.2), 44 px at phone width.
export const SELECT_CLASS =
	"h-8 rounded-lg border border-border-strong bg-surface-sunken px-2.5 text-sm text-text max-md:h-11";

// One card of the settings page: a surface with its title as an h2 and an id the
// section list on the left links to. Every word is the caller's.
export function SettingsSection({
	id,
	title,
	description,
	children,
	className,
	"data-testid": testId,
}: {
	id: string;
	title: string;
	description?: string;
	children: ReactNode;
	className?: string;
	"data-testid"?: string;
}): ReactNode {
	return (
		<section
			id={id}
			aria-labelledby={`${id}-title`}
			data-testid={testId}
			className={cn(
				"flex scroll-mt-4 flex-col gap-4 rounded-lg border border-border bg-surface p-5 text-text",
				className,
			)}
		>
			<div>
				<h2 id={`${id}-title`} className="text-xl font-semibold">
					{title}
				</h2>
				{description ? (
					<p className="mt-1 text-sm text-text-muted">{description}</p>
				) : null}
			</div>
			{children}
		</section>
	);
}
