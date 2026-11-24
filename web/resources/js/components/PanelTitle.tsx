import type { ReactNode } from "react";

// The title strip of a panel of an editor: it stays in view while the body of the panel scrolls, so the
// person always knows which panel they are in. The panel is the scrolling parent and carries 12 px of padding,
// which the strip takes back with its negative margins. The video editor (material panels) and the image
// editor (layers, inspector) share it. A sticky strip sticks at the padding edge of its scroll parent, so its
// offset is -top-3 and not top-0: at rest it sits flush with the top, and scrolled it still covers the 12 px
// the padding would leave open above it.
export default function PanelTitle({
	children,
}: {
	children: ReactNode;
}): ReactNode {
	return (
		<h2 className="sticky -top-3 z-10 -mx-3 -mt-3 mb-1 flex shrink-0 items-center gap-2 border-b border-border bg-ed-panel px-3 py-2 text-sm font-semibold text-foreground">
			{children}
		</h2>
	);
}
