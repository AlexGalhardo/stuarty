import type { LucideIcon } from "lucide-react";
import type { ReactNode } from "react";

// The title strip of a panel of the image editor, as the final design draws it: a 16 px icon and the name, on a
// rule. It is editor-local (the video editor has its own); swap it for the shared panel header when group 1
// provides one.
export default function PanelHeader({
	icon: Icon,
	children,
}: {
	icon: LucideIcon;
	children: ReactNode;
}): ReactNode {
	return (
		<h2 className="m-0 flex h-9 shrink-0 items-center gap-2 border-b border-border bg-ed-panel px-3 text-xs font-semibold text-foreground">
			<Icon
				aria-hidden="true"
				className="size-4 shrink-0 text-muted-foreground"
			/>
			{children}
		</h2>
	);
}
