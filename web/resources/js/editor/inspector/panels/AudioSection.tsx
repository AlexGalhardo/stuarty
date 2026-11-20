import type { ReactNode } from "react";

import { useT } from "@/lib/i18n";
import {
	audioOperations,
	VOICE_PRESETS,
	type VoicePreset,
} from "../../commands/builders";
import { registryLimits, registryRange } from "../../commands/registryLimits";
import { formatTime, fpsOf } from "../../commands/units";
import { useCanEdit } from "../../commands/useActions";
import { useDocumentState } from "../../state/EditorContext";
import { readTimeDisplay } from "../../state/prefs";
import NumberField from "../fields/NumberField";
import SelectField from "../fields/SelectField";
import SliderField from "../fields/SliderField";
import TimeCommitField from "../fields/TimeCommitField";
import ToggleField from "../fields/ToggleField";
import Section from "../Section";
import { useFieldCommit } from "../useFieldCommit";

const isVoice = (value: string): value is VoicePreset =>
	(VOICE_PRESETS as readonly string[]).includes(value);

// The Audio section of SDD-012 section 12.1 for a clip, an audio item and a composition:
// volume, the two fades, the noise reduction, the voice filter and, for a clip and a composition, the mute flag.
// A composition has no noise reduction and no voice filter (its sound is the mix of the nested timeline; the core
// refuses them), so they are not offered. Each field is one `item.set_audio` with only the field that changed.
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
	const composition = item.type === "composition";
	return (
		<Section title={t("ui.editor.clip.audio_title")}>
			<NumberField
				label={t("ui.editor.clip.volume")}
				stored={String(item.volume ?? 1)}
				disabled={!canEdit}
				step={0.1}
				{...registryLimits("item.set_audio", "volume")}
				commit={(volume) => send(audioOperations(itemId, { volume }))}
			/>
			<TimeCommitField
				label={t("ui.editor.clip.fade_in")}
				stored={formatTime(item.fade_in ?? 0, fps, readTimeDisplay())}
				disabled={!canEdit}
				commit={(fade_in) => send(audioOperations(itemId, { fade_in }))}
			/>
			<TimeCommitField
				label={t("ui.editor.clip.fade_out")}
				stored={formatTime(item.fade_out ?? 0, fps, readTimeDisplay())}
				disabled={!canEdit}
				commit={(fade_out) => send(audioOperations(itemId, { fade_out }))}
			/>
			{!composition && (
				<>
					<SliderField
						label={t("ui.editor.clip.denoise")}
						stored={String(item.denoise ?? 0)}
						disabled={!canEdit}
						{...registryRange("item.set_audio", "denoise")}
						step={0.01}
						commit={(denoise) => send(audioOperations(itemId, { denoise }))}
					/>
					<SelectField
						label={t("ui.editor.clip.voice")}
						stored={item.voice ?? "none"}
						disabled={!canEdit}
						options={VOICE_PRESETS.map((preset) => ({
							value: preset,
							label: t(`ui.editor.clip.voice_${preset}`),
						}))}
						commit={async (value) =>
							isVoice(value)
								? send(audioOperations(itemId, { voice: value }))
								: { ok: true }
						}
					/>
				</>
			)}
			{(item.type === "clip" || composition) && (
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
