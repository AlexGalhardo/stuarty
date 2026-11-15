import type { ReactNode } from "react";

import CaptionStyle from "./CaptionStyle";
import CaptionWords from "./CaptionWords";
import ItemSections from "./ItemSections";

// The caption panel: Name, Timing and Transform, then the word list and the
// caption style (SDD-012 section 12.1).
export default function CaptionPanel({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	return (
		<>
			<ItemSections itemId={itemId} />
			<CaptionWords itemId={itemId} />
			<CaptionStyle itemId={itemId} />
		</>
	);
}
