import type { ReactNode } from "react";

// The title strip of a material panel: it stays in view while the body scrolls, so the person always
// knows which panel they are in.
export function PanelTitle({ children }: { children: ReactNode }): ReactNode {
	return (
		<h2 className="sticky top-0 z-10 -mx-3 -mt-3 mb-1 border-b border-border bg-ed-panel px-3 py-2 text-sm font-semibold text-foreground">
			{children}
		</h2>
	);
}

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
			<PanelTitle>{title}</PanelTitle>
			{children}
		</section>
	);
}
