import type { ReactNode } from "react";

import { useDocumentState } from "../../state/EditorContext";
import AudioSection from "./AudioSection";
import AutoCaptions from "./AutoCaptions";
import ChromaKeySection from "./ChromaKeySection";
import ColorSection from "./ColorSection";
import FiltersSection from "./FiltersSection";
import FramingSection from "./FramingSection";
import ItemSections from "./ItemSections";
import SilencesSection from "./SilencesSection";
import SpeedSection from "./SpeedSection";

// The clip panel (SDD-012 section 12.1): the sections every item has, then
// audio, framing, speed, colour and silences, then auto captions. A still image
// has no sound and no speed, so those sections are left out for it.
export default function ClipPanel({ itemId }: { itemId: string }): ReactNode {
	const still = useDocumentState((s) => {
		const asset = s.document?.items[itemId]?.asset;
		return asset !== undefined && s.document?.assets[asset]?.kind === "image";
	});
	return (
		<>
			<ItemSections itemId={itemId} />
			{!still && <AudioSection itemId={itemId} />}
			<FramingSection itemId={itemId} />
			{!still && <SpeedSection itemId={itemId} />}
			<ColorSection itemId={itemId} />
			<FiltersSection itemId={itemId} />
			<ChromaKeySection itemId={itemId} />
			<SilencesSection itemId={itemId} />
			<AutoCaptions itemId={itemId} />
		</>
	);
}
