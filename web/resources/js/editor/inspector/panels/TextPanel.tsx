import type { ReactNode } from "react";

import ItemSections from "./ItemSections";
import TextSection from "./TextSection";

// The text panel: Name, Timing and Transform, then the Text section (SDD-012
// section 12.1).
export default function TextPanel({ itemId }: { itemId: string }): ReactNode {
	return (
		<>
			<ItemSections itemId={itemId} />
			<TextSection itemId={itemId} />
		</>
	);
}
