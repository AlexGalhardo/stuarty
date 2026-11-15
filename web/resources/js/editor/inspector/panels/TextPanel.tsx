import type { ReactNode } from "react";

import ItemSections from "./ItemSections";

// The text panel: Name and Timing today; the text style sections are added by
// the task that builds them (SDD-012 section 12.1).
export default function TextPanel({ itemId }: { itemId: string }): ReactNode {
	return <ItemSections itemId={itemId} />;
}
