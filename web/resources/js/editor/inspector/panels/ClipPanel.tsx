import type { ReactNode } from "react";

import { useDocumentState } from "../../state/EditorContext";
import InspectorTabs from "../InspectorTabs";
import AudioProcessors from "./AudioProcessors";
import AudioSection from "./AudioSection";
import AutoCaptions from "./AutoCaptions";
import ChromaKeySection from "./ChromaKeySection";
import ColorSection from "./ColorSection";
import EffectsSection from "./EffectsSection";
import FiltersSection from "./FiltersSection";
import FramingSection from "./FramingSection";
import {
	ItemAnimation,
	ItemBasics,
	ItemKeyframes,
	ItemVisual,
} from "./ItemSections";
import OutlineSection from "./OutlineSection";
import ShadowSection from "./ShadowSection";
import SilencesSection from "./SilencesSection";
import SpeedSection from "./SpeedSection";

// The clip panel (SDD-012 sections 6.4 and 12.1), in tabs: Video (name, timing,
// transform, framing, keyframes, transitions), Audio, Speed, Animation, Adjust
// (colour, filters, chroma key, effects) and Text (auto captions, only for a
// clip with sound). A still image has no sound and no speed, so those tabs are
// left out for it.
export default function ClipPanel({ itemId }: { itemId: string }): ReactNode {
	const still = useDocumentState((s) => {
		const asset = s.document?.items[itemId]?.asset;
		return asset !== undefined && s.document?.assets[asset]?.kind === "image";
	});
	const captionable = useDocumentState((s) => {
		const asset = s.document?.items[itemId]?.asset;
		const found = asset === undefined ? undefined : s.document?.assets[asset];
		return found?.kind === "video" && found.has_audio !== false;
	});
	return (
		<InspectorTabs
			tabs={[
				{
					id: "video",
					content: (
						<>
							<ItemBasics itemId={itemId} />
							<ItemVisual itemId={itemId} />
							<FramingSection itemId={itemId} />
							<ShadowSection itemId={itemId} />
							<OutlineSection itemId={itemId} />
							<ItemKeyframes itemId={itemId} />
						</>
					),
				},
				...(still
					? []
					: [
							{
								id: "audio" as const,
								content: (
									<>
										<AudioSection itemId={itemId} />
										<AudioProcessors target={{ itemId }} />
										<SilencesSection itemId={itemId} />
									</>
								),
							},
							{
								id: "speed" as const,
								content: <SpeedSection itemId={itemId} />,
							},
						]),
				{ id: "animation", content: <ItemAnimation itemId={itemId} /> },
				{
					id: "adjust",
					content: (
						<>
							<ColorSection itemId={itemId} />
							<FiltersSection itemId={itemId} />
							<ChromaKeySection itemId={itemId} />
							<EffectsSection itemId={itemId} />
						</>
					),
				},
				...(captionable
					? [
							{
								id: "text" as const,
								content: <AutoCaptions itemId={itemId} />,
							},
						]
					: []),
			]}
		/>
	);
}
