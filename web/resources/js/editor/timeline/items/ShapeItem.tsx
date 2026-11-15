import { Circle, RectangleHorizontal, Square } from "lucide-react";
import type { ReactNode } from "react";

import { useDocumentState } from "../../state/EditorContext";

const ICONS = {
	rect: Square,
	rounded_rect: RectangleHorizontal,
	ellipse: Circle,
} as const;

// The fill swatch and the shape icon of a shape item (SDD-012 section 7.4),
// read from the read model. The colour is the user's own content.
export default function ShapeItem({ id }: { id: string }): ReactNode {
	const fill = useDocumentState((s) => s.document?.items[id]?.fill);
	const shape = useDocumentState((s) => s.document?.items[id]?.shape);
	const Icon = ICONS[shape ?? "rect"];
	return (
		<span className="relative z-10 mr-1 flex shrink-0 items-center gap-1">
			<span
				aria-hidden="true"
				data-shape-swatch=""
				className="size-3.5 rounded-sm border border-foreground/40"
				style={{ backgroundColor: fill ?? "transparent" }}
			/>
			<Icon aria-hidden="true" className="size-3.5" />
		</span>
	);
}
