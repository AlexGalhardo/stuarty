import type { ReactNode } from "react";

import ItemSections from "./ItemSections";

// The shape panel: Name and Timing today; the shape fields are added by the
// task that builds them (SDD-012 section 12.1).
export default function ShapePanel({ itemId }: { itemId: string }): ReactNode {
	return <ItemSections itemId={itemId} />;
}
