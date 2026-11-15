import type { ReactNode } from "react";

import ItemSections from "./ItemSections";
import ShapeSection from "./ShapeSection";

// The shape panel (SDD-012 section 12.1): the sections every item has, then
// the shape fields.
export default function ShapePanel({ itemId }: { itemId: string }): ReactNode {
	return (
		<>
			<ItemSections itemId={itemId} />
			<ShapeSection itemId={itemId} />
		</>
	);
}
