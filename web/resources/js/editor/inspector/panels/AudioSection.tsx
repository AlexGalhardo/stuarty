import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import { audioOperations } from "../../commands/builders";
import { formatTime, fpsOf } from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import { DISPLAY } from "../display";
import NumberField from "../fields/NumberField";
import TimeCommitField from "../fields/TimeCommitField";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

// The Audio section of SDD-012 section 12.1 for a clip and an audio item:
// volume, the two fades and, for a clip, the mute flag. Each field is one
// `item.set_audio` with only the field that changed.
export default function AudioSection({
	itemId,
}: {
	itemId: string;
}): ReactNode {
	const t = useT();
	const canEdit = useCanEdit();
	const send = useFieldCommit();
	const item = useDocumentState((s) => s.document?.items[itemId]);
	const rate = useDocumentState((s) => s.document?.settings.fps);
	if (item === undefined) return null;
	const fps = fpsOf(rate);
	return (
		<Section title={t("ui.editor.clip.audio_title")}>
			<NumberField
				label={t("ui.editor.clip.volume")}
				stored={String(item.volume ?? 1)}
				disabled={!canEdit}
				step={0.1}
				min={0}
				max={4}
				commit={(volume) => send(audioOperations(itemId, { volume }))}
			/>
			<TimeCommitField
				label={t("ui.editor.clip.fade_in")}
				stored={formatTime(item.fade_in ?? 0, fps, DISPLAY)}
				disabled={!canEdit}
				commit={(fade_in) => send(audioOperations(itemId, { fade_in }))}
			/>
			<TimeCommitField
				label={t("ui.editor.clip.fade_out")}
				stored={formatTime(item.fade_out ?? 0, fps, DISPLAY)}
				disabled={!canEdit}
				commit={(fade_out) => send(audioOperations(itemId, { fade_out }))}
			/>
			{item.type === "clip" && (
				<ToggleField
					label={t("ui.editor.clip.mute")}
					stored={item.audio_muted === true}
					disabled={!canEdit}
					commit={(audio_muted) =>
						send(audioOperations(itemId, { audio_muted }))
					}
				/>
			)}
		</Section>
	);
}
