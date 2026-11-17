import type { ReactNode } from "react";

import { useDocumentState } from "../../state/EditorContext";
import InspectorTabs from "../InspectorTabs";
import CaptionStyle from "./CaptionStyle";
import CaptionWords from "./CaptionWords";
import ImportSubtitles from "./ImportSubtitles";
import {
	ItemAnimation,
	ItemBasics,
	ItemKeyframes,
	ItemVisual,
} from "./ItemSections";

// The caption panel (SDD-012 sections 6.4 and 12.1): Video, Animation and Text (the words, the style and the subtitle import).
export default function CaptionPanel({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const track = useDocumentState((s) => s.document?.items[itemId]?.track);
	return (
		<InspectorTabs
			tabs={[
				{
					id: "video",
					content: (
						<>
							<ItemBasics itemId={itemId} />
							<ItemVisual itemId={itemId} />
							<ItemKeyframes itemId={itemId} />
						</>
					),
				},
				{ id: "animation", content: <ItemAnimation itemId={itemId} /> },
				{
					id: "text",
					content: (
						<>
							<CaptionWords itemId={itemId} />
							<CaptionStyle itemId={itemId} />
							{track !== undefined && <ImportSubtitles trackId={track} />}
						</>
					),
				},
			]}
		/>
	);
}
