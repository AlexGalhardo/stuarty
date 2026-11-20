import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { type TrackChanges, trackUpdateOperations } from "../commands/builders";
import { useCanEdit } from "../commands/useActions";
import { useDocumentState } from "../state/EditorContext";
import TextField from "./fields/TextField";
import ToggleField from "./fields/ToggleField";
import AudioProcessors from "./panels/AudioProcessors";
import ImportSubtitles from "./panels/ImportSubtitles";
import TrackCaptionStyle from "./panels/TrackCaptionStyle";
import { useFieldCommit } from "./useFieldCommit";

// A track selected: its panel (SDD-012 section 12.1, row Track panel). Name,
// hidden, muted and magnet send `track.update`; the caption style sends
// `track.set_caption_style`, merged by default, or as the whole style while
// "Replace" is on.
export default function TrackPanel({
	trackId,
}: {
	trackId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const track = useDocumentState((s) => s.document?.tracks[trackId]);
	if (track === undefined) return null;
	const update = (changes: TrackChanges) =>
		send(trackUpdateOperations(trackId, changes));
	return (
		<div className="flex flex-col gap-3">
			<TextField
				label={t("ui.editor.inspector.name")}
				stored={track.name ?? ""}
				disabled={!canEdit}
				maxLength={60}
				commit={(name) => update({ name })}
			/>
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.inspector.kind")}:{" "}
				{t(`ui.editor.inspector.track_kind.${track.kind}`)}
			</p>
			{track.kind === "visual" && (
				<ToggleField
					label={t("ui.editor.track.hidden")}
					stored={track.hidden === true}
					disabled={!canEdit}
					commit={(hidden) => update({ hidden })}
				/>
			)}
			<ToggleField
				label={t("ui.editor.track.muted")}
				stored={track.muted === true}
				disabled={!canEdit}
				commit={(muted) => update({ muted })}
			/>
			<ToggleField
				label={t("ui.editor.track.magnet")}
				stored={track.magnet === true}
				disabled={!canEdit}
				commit={(magnet) => update({ magnet })}
			/>
			<AudioProcessors target={{ trackId }} />
			<TrackCaptionStyle trackId={trackId} />
			{track.kind === "visual" && <ImportSubtitles trackId={trackId} />}
		</div>
	);
}
