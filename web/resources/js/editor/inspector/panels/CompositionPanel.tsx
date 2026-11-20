import type { ReactNode } from "react";

import InspectorTabs from "../InspectorTabs";
import AudioSection from "./AudioSection";
import { ItemBasics, ItemKeyframes, ItemVisual } from "./ItemSections";
import OutlineSection from "./OutlineSection";
import ShadowSection from "./ShadowSection";
import SpeedSection from "./SpeedSection";

// The panel of a composition item (SDD-012 sections 6.4 and 12.1, epics 53
// and VV-288): the Video tab has name, timing, transform, flip, blend, mask,
// transitions, shadow, outline and keyframes, the Audio tab its volume, fades
// and mute (the sound is the mix of the nested timeline, so no noise
// reduction, voice filter, processors or silences), and the Speed tab a
// constant speed only. The core refuses a crop, the colour and the chroma key
// on a composition; its fit, filter, blur and effects, which the core takes
// (SDD-001 section 3.5.1), are not offered here yet.
export default function CompositionPanel({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	return (
		<InspectorTabs
			tabs={[
				{
					id: "video",
					content: (
						<>
							<ItemBasics itemId={itemId} />
							<ItemVisual itemId={itemId} />
							<ShadowSection itemId={itemId} />
							<OutlineSection itemId={itemId} />
							<ItemKeyframes itemId={itemId} />
						</>
					),
				},
				{ id: "audio", content: <AudioSection itemId={itemId} /> },
				{
					id: "speed",
					content: <SpeedSection itemId={itemId} constantOnly />,
				},
			]}
		/>
	);
}
