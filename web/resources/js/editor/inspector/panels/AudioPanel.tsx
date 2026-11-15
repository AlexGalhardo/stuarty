import type { ReactNode } from "react";

import AudioSection from "./AudioSection";
import ItemSections from "./ItemSections";

// The audio panel (SDD-012 section 12.1): Name and Timing, then the audio
// section.
export default function AudioPanel({ itemId }: { itemId: string }): ReactNode {
	return (
		<>
			<ItemSections itemId={itemId} />
			<AudioSection itemId={itemId} />
		</>
	);
}
