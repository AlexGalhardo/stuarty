import type { ReactNode } from "react";

// The frame of a material panel (docs/design-system.md 3.4.4): a region named
// after its tab, with a header and a body that scrolls on its own.
export default function PanelFrame({
	title,
	children,
}: {
	title: string;
	children: ReactNode;
}): ReactNode {
	return (
		<section
			aria-label={title}
			className="flex min-h-0 min-w-0 flex-col gap-2 overflow-auto bg-ed-panel p-3"
		>
			<h2 className="text-xs font-medium tracking-wide text-muted-foreground uppercase">
				{title}
			</h2>
			{children}
		</section>
	);
}
