import type { ReactNode } from "react";

import ItemSections from "./ItemSections";

// The audio panel: Name and Timing today; its audio section is added by the
// task that builds it (SDD-012 section 12.1).
export default function AudioPanel({ itemId }: { itemId: string }): ReactNode {
	return <ItemSections itemId={itemId} />;
}
