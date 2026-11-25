import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { magnetTrackId } from "../commands/magnet";
import InspectorTabs from "../inspector/InspectorTabs";
import CaptionStyle from "../inspector/panels/CaptionStyle";
import CaptionWords from "../inspector/panels/CaptionWords";
import { ItemBasics } from "../inspector/panels/ItemSections";
import TrackCaptionStyle from "../inspector/panels/TrackCaptionStyle";
import { useDocumentState, useUiState } from "../state/EditorContext";

// The inspector of the captions screen (final design 28): the tabs Style, Text and Time over the caption track the
// Captions tab of the rail acts on. Style is the gallery of the named styles of the core and the style rows of the
// track; Text is the words of the selected caption; Time is its start and length. Every control sends the command
// it always did (`caption.apply_style`, `track.set_caption_style`, `caption.set_style`, `caption.edit_word`,
// `item.trim`, `item.move`).
export default function CaptionsInspector(): ReactNode {
	const t = useT();
	const selection = useUiState((s) => s.selection);
	const focusedItem = useUiState((s) => s.focusedItem);
	const only = selection.items.length === 1 ? selection.items[0] : undefined;
	const caption = useDocumentState((s) =>
		only !== undefined && s.document?.items[only]?.type === "caption"
			? only
			: undefined,
	);
	const trackId = useDocumentState((s) =>
		magnetTrackId(s, { selection, focusedItem }),
	);
	const trackKind = useDocumentState((s) =>
		trackId === null ? undefined : s.document?.tracks[trackId]?.kind,
	);
	const hint = (key: string): ReactNode => (
		<p className="text-xs text-muted-foreground">{t(key)}</p>
	);
	return (
		<InspectorTabs
			tabs={[
				{
					id: "style",
					content: (
						<>
							{caption !== undefined && <CaptionStyle itemId={caption} />}
							{trackId !== null && trackKind === "visual" ? (
								<TrackCaptionStyle trackId={trackId} />
							) : (
								hint("ui.editor.captions_inspector.no_track")
							)}
						</>
					),
				},
				{
					id: "words",
					content:
						caption === undefined ? (
							hint("ui.editor.captions_inspector.select_caption")
						) : (
							<CaptionWords itemId={caption} />
						),
				},
				{
					id: "timing",
					content:
						caption === undefined ? (
							hint("ui.editor.captions_inspector.select_caption")
						) : (
							<ItemBasics itemId={caption} />
						),
				},
			]}
		/>
	);
}
