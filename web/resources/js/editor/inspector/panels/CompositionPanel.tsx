import type { ReactNode } from "react";

import InspectorTabs from "../InspectorTabs";
import AudioSection from "./AudioSection";
import { ItemBasics, ItemKeyframes, ItemVisual } from "./ItemSections";
import OutlineSection from "./OutlineSection";
import ShadowSection from "./ShadowSection";
import SpeedSection from "./SpeedSection";

// The panel of a composition item (SDD-012 sections 6.4 and 12.1, epics 53
// and VV-288): the Video tab has what the core takes on a composition (name,
// timing, transform, flip, blend, mask, transitions, shadow, outline and
// keyframes), the Audio tab its volume, fades and mute (the sound is the mix
// of the nested timeline, so no noise reduction, voice filter, processors or
// silences), and the Speed tab a constant speed only. Framing, colour and
// chroma key are for a clip: the core refuses them on a composition.
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
