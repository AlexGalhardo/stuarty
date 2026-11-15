import type { ReactNode } from "react";

import AutoCaptions from "./AutoCaptions";
import ItemSections from "./ItemSections";

// The clip panel: Name and Timing today; the sections of a clip (transform,
// audio, framing, speed, colour, silences) are added by the tasks that build
// them (SDD-012 section 12.1). Auto captions are the section of a clip with
// audio.
export default function ClipPanel({ itemId }: { itemId: string }): ReactNode {
	return (
		<>
			<ItemSections itemId={itemId} />
			<AutoCaptions itemId={itemId} />
		</>
	);
}
