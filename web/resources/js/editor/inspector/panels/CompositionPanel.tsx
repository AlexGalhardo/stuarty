import type { ReactNode } from "react";

import InspectorTabs from "../InspectorTabs";
import { ItemBasics } from "./ItemSections";
import SpeedSection from "./SpeedSection";

// The panel of a composition item (SDD-012 section 12.1, epic 53): its name
// and timing, and the Speed section with a constant speed only. The core
// refuses a curve and a reverse on a composition, so they are not offered.
export default function CompositionPanel({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	return (
		<InspectorTabs
			tabs={[
				{ id: "video", content: <ItemBasics itemId={itemId} /> },
				{
					id: "speed",
					content: <SpeedSection itemId={itemId} constantOnly />,
				},
			]}
		/>
	);
}
