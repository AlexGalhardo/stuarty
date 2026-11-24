import type { ReactNode } from "react";

import PanelTitle from "@/components/PanelTitle";

// The title strip moved to components/ when the image editor began to use it (epic 86); the material panels
// keep importing it from here.
export { PanelTitle };

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
