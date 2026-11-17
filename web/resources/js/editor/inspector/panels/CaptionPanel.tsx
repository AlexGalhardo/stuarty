import type { ReactNode } from "react";

import { useDocumentState } from "../../state/EditorContext";
import CaptionStyle from "./CaptionStyle";
import CaptionWords from "./CaptionWords";
import ImportSubtitles from "./ImportSubtitles";
import ItemSections from "./ItemSections";

// The caption panel: Name, Timing and Transform, then the word list and the
// caption style (SDD-012 section 12.1).
export default function CaptionPanel({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const track = useDocumentState((s) => s.document?.items[itemId]?.track);
	return (
		<>
			<ItemSections itemId={itemId} />
			<CaptionWords itemId={itemId} />
			<CaptionStyle itemId={itemId} />
			{track !== undefined && <ImportSubtitles trackId={track} />}
		</>
	);
}
