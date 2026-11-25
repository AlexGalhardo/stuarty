import type { LucideIcon } from "lucide-react";
import { createContext, type ReactNode, useContext } from "react";

import SharedPanelTitle from "@/components/PanelTitle";

// The icon of the tab of the rail that shows the panel: the title of the panel repeats it, as the final design
// draws it, so the person sees which panel the rail opened. LeftPanel provides it; a panel outside the rail
// (the history) has none.
export const TabIcon = createContext<LucideIcon | null>(null);

// The title strip of a material panel: the shared strip (components/PanelTitle) with the icon of its tab.
export function PanelTitle({ children }: { children: ReactNode }): ReactNode {
	const Icon = useContext(TabIcon);
	return (
		<SharedPanelTitle>
			{Icon !== null && (
				<Icon aria-hidden="true" className="size-4 text-muted-foreground" />
			)}
			{children}
		</SharedPanelTitle>
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
