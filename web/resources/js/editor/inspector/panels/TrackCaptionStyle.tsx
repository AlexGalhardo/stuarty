import { type ReactNode, useId, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import { trackCaptionStyleOperations } from "../../commands/builders";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";
import StyleRows, { CAPTION_STYLE_ROWS } from "./StyleRows";

// The caption style of a track (SDD-012 section 12.1, row Track panel):
// `track.set_caption_style`, merged by default, or as the whole style while
// "Replace" is on. Shown in the track panel and in the Captions tab.
export default function TrackCaptionStyle({
	trackId,
}: {
	trackId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const replaceId = useId();
	const [replace, setReplace] = useState(false);
	const track = useDocumentState((s) => s.document?.tracks[trackId]);
	if (track === undefined) return null;
	return (
		<Section title={t("ui.editor.inspector.caption_style_title")}>
			{track.kind === "audio" ? (
				<p className="text-xs text-muted-foreground">
					{t("ui.editor.track.no_caption_style")}
				</p>
			) : (
				<>
					<StyleRows
						rows={CAPTION_STYLE_ROWS}
						stored={track.caption_style ?? {}}
						disabled={!canEdit}
						commit={(changes) =>
							send(trackCaptionStyleOperations(trackId, changes, replace))
						}
					/>
					<div className="flex items-center gap-2 text-xs">
						<input
							id={replaceId}
							type="checkbox"
							checked={replace}
							disabled={!canEdit}
							className="size-4 accent-primary"
							onChange={(event) => setReplace(event.target.checked)}
						/>
						<label htmlFor={replaceId}>
							{t("ui.editor.track.caption_style_replace")}
						</label>
					</div>
					<Button
						type="button"
						variant="outline"
						size="sm"
						disabled={!canEdit || track.caption_style === undefined}
						onClick={() =>
							void send(trackCaptionStyleOperations(trackId, {}, true))
						}
					>
						{t("ui.editor.track.caption_style_clear")}
					</Button>
				</>
			)}
		</Section>
	);
}
