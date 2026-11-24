import { type ReactNode, useState } from "react";

import { Button } from "@/components/ui/button";
import { useT } from "@/lib/i18n";
import {
	applyVoicePresetOperations,
	type ProcessorTarget,
} from "../../commands/processorBuilders";
import {
	VOICE_PRESETS,
	type VoicePresetId,
} from "../../commands/processorTable";
import { useActions, useCanEdit } from "../../commands/useActions";
import { useCommandsExposed } from "../../commands/useExposedCommands";
import SelectField from "../fields/SelectField";

// The studio voice action of an item or a track (SDD-002 C.91): one choice and one button for a clean, even
// voice. The core replaces the chain with the preset; the processors are edited below as before, and the export
// brings the mix to the loudness target of the project.
export default function VoicePreset({
	target,
}: {
	target: ProcessorTarget;
}): ReactNode {
	const t = useT();
	const actions = useActions();
	const canEdit = useCanEdit();
	const exposed = useCommandsExposed("audio.apply_voice_preset");
	const [preset, setPreset] = useState<VoicePresetId | undefined>(undefined);
	const chosen = preset ?? VOICE_PRESETS[0];
	if (!exposed || chosen === undefined) return null;
	return (
		<div className="flex flex-col gap-2" data-testid="voice-preset">
			<p className="text-xs text-muted-foreground">
				{t("ui.editor.voice_preset.hint")}
			</p>
			<SelectField
				label={t("ui.editor.voice_preset.label")}
				stored={chosen}
				disabled={!canEdit}
				options={VOICE_PRESETS.map((id) => ({
					value: id,
					label: t(`ui.editor.voice_preset.presets.${id}`),
				}))}
				commit={async (value) => {
					setPreset(value as VoicePresetId);
					return { ok: true };
				}}
			/>
			<Button
				type="button"
				variant="default"
				size="sm"
				disabled={!canEdit}
				onClick={() =>
					void actions.runOperations(applyVoicePresetOperations(target, chosen))
				}
			>
				{t("ui.editor.voice_preset.apply")}
			</Button>
		</div>
	);
}
