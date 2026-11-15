import type { ReactNode } from "react";

import ItemSections from "./ItemSections";

// The caption panel: Name and Timing today; the word list and the caption
// style are added by the tasks that build them (SDD-012 section 12.1).
export default function CaptionPanel({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	return <ItemSections itemId={itemId} />;
}
