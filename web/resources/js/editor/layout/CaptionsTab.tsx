import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { magnetTrackId } from "../commands/magnet";
import AutoCaptions from "../inspector/panels/AutoCaptions";
import CaptionStyle from "../inspector/panels/CaptionStyle";
import ImportSubtitles from "../inspector/panels/ImportSubtitles";
import TrackCaptionStyle from "../inspector/panels/TrackCaptionStyle";
import PanelFrame from "../library/PanelFrame";
import { useDocumentState, useUiState } from "../state/EditorContext";

// The Captions tab of the material rail (SDD-012 section 6.4). It reuses the
// sections of the inspector, so every control sends the command it always did:
// `caption.from_transcript` (through the transcription), `caption.import_subtitles`,
// `caption.set_style` and `track.set_caption_style`.
export default function CaptionsTab(): ReactNode {
	const t = useT();
	const selection = useUiState((s) => s.selection);
	const focusedItem = useUiState((s) => s.focusedItem);
	const only = selection.items.length === 1 ? selection.items[0] : undefined;
	const type = useDocumentState((s) =>
		only === undefined ? undefined : s.document?.items[only]?.type,
	);
	const trackId = useDocumentState((s) =>
		magnetTrackId(s, { selection, focusedItem }),
	);
	const trackKind = useDocumentState((s) =>
		trackId === null ? undefined : s.document?.tracks[trackId]?.kind,
	);
	return (
		<PanelFrame title={t("ui.editor.panels.captions")}>
			{only !== undefined && type === "clip" ? (
				<AutoCaptions itemId={only} />
			) : (
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.library.captions_select_clip")}
				</p>
			)}
			{trackId !== null && trackKind === "visual" && (
				<ImportSubtitles trackId={trackId} />
			)}
			{only !== undefined && type === "caption" ? (
				<CaptionStyle itemId={only} />
			) : (
				trackId !== null && <TrackCaptionStyle trackId={trackId} />
			)}
		</PanelFrame>
	);
}
