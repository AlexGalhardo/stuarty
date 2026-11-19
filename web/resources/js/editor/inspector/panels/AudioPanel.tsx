import type { ReactNode } from "react";

import InspectorTabs from "../InspectorTabs";
import AudioProcessors from "./AudioProcessors";
import AudioSection from "./AudioSection";
import BeatMarkers from "./BeatMarkers";
import DuckingSection from "./DuckingSection";
import { ItemBasics, ItemKeyframes } from "./ItemSections";

// The audio panel (SDD-012 sections 6.4 and 12.1): Video (name, timing,
// keyframes, markers) and Audio (volume, ducking, beat markers).
export default function AudioPanel({ itemId }: { itemId: string }): ReactNode {
	return (
		<InspectorTabs
			tabs={[
				{
					id: "video",
					content: (
						<>
							<ItemBasics itemId={itemId} />
							<ItemKeyframes itemId={itemId} />
						</>
					),
				},
				{
					id: "audio",
					content: (
						<>
							<AudioSection itemId={itemId} />
							<AudioProcessors target={{ itemId }} />
							<DuckingSection itemId={itemId} />
							<BeatMarkers itemId={itemId} />
						</>
					),
				},
			]}
		/>
	);
}
